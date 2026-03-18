// state-patcher modifies contract storage slots in a geth PebbleDB database.
//
// Built against go-ethereum v1.16.8. Supports both hash and path (PBSS) state schemes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb/pebble"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/ethereum/go-ethereum/triedb/hashdb"
	"github.com/ethereum/go-ethereum/triedb/pathdb"
	"gopkg.in/yaml.v3"
)

// Patch represents a single storage slot modification.
type Patch struct {
	Address common.Address
	Slot    common.Hash
	Value   common.Hash
}

// setFlags collects multiple --set flag values.
type setFlags []string

func (s *setFlags) String() string { return strings.Join(*s, ", ") }
func (s *setFlags) Set(val string) error {
	*s = append(*s, val)
	return nil
}

// parsePatch parses "0xAddr:0xSlot=0xValue" into a Patch.
func parsePatch(raw string) (Patch, error) {
	colon := strings.Index(raw, ":")
	if colon < 0 {
		return Patch{}, fmt.Errorf("missing ':' in %q, expected 0xAddr:0xSlot=0xValue", raw)
	}
	eq := strings.Index(raw[colon+1:], "=")
	if eq < 0 {
		return Patch{}, fmt.Errorf("missing '=' in %q, expected 0xAddr:0xSlot=0xValue", raw)
	}
	eq += colon + 1

	addrHex := raw[:colon]
	slotHex := raw[colon+1 : eq]
	valHex := raw[eq+1:]

	if !common.IsHexAddress(addrHex) {
		return Patch{}, fmt.Errorf("invalid address: %s", addrHex)
	}

	return Patch{
		Address: common.HexToAddress(addrHex),
		Slot:    common.HexToHash(slotHex),
		Value:   common.HexToHash(valHex),
	}, nil
}

// loadPatchFile loads patches from a JSON or YAML file.
// Format: { "0xAddr": { "0xSlot": "0xValue", ... }, ... }
func loadPatchFile(path string) ([]Patch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// map[address]map[slot]value
	var raw map[string]map[string]string

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported file extension %q (use .json, .yaml, or .yml)", ext)
	}

	var patches []Patch
	for addrHex, slots := range raw {
		if !common.IsHexAddress(addrHex) {
			return nil, fmt.Errorf("invalid address in file: %s", addrHex)
		}
		addr := common.HexToAddress(addrHex)
		for slotHex, valHex := range slots {
			patches = append(patches, Patch{
				Address: addr,
				Slot:    common.HexToHash(slotHex),
				Value:   common.HexToHash(valHex),
			})
		}
	}
	return patches, nil
}

func main() {
	var (
		datadir   string
		sets      setFlags
		patchFile string
		dryRun    bool
	)
	flag.StringVar(&datadir, "datadir", "", "geth data directory (required)")
	flag.Var(&sets, "set", "storage patch: 0xAddr:0xSlot=0xValue (repeatable)")
	flag.StringVar(&patchFile, "file", "", "patch file in JSON or YAML format")
	flag.BoolVar(&dryRun, "dry-run", false, "validate inputs only, do not modify database")
	flag.Parse()

	if datadir == "" || (len(sets) == 0 && patchFile == "") {
		fmt.Fprintf(os.Stderr, "Usage: state-patcher --datadir <path> [--set 0xAddr:0xSlot=0xValue ...] [--file patch.json|patch.yaml]\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var patches []Patch
	if patchFile != "" {
		fp, err := loadPatchFile(patchFile)
		if err != nil {
			log.Fatalf("bad --file %q: %v", patchFile, err)
		}
		patches = append(patches, fp...)
	}
	for _, s := range sets {
		p, err := parsePatch(s)
		if err != nil {
			log.Fatalf("bad --set %q: %v", s, err)
		}
		patches = append(patches, p)
	}

	fmt.Println("=== state-patcher ===")
	fmt.Printf("datadir: %s\n", datadir)
	for i, p := range patches {
		fmt.Printf("patch[%d]: addr=%s slot=%s val=%s\n",
			i, p.Address.Hex(), p.Slot.Hex(), p.Value.Hex())
	}

	if dryRun {
		fmt.Println("\ndry-run: inputs OK, exiting without changes")
		return
	}

	if err := patchState(datadir, patches); err != nil {
		log.Fatalf("FATAL: %v", err)
	}
	fmt.Println("\n=== state patching complete ===")
}

// patchState opens the geth database, modifies storage slots, and updates
// the head block header so that the new state root (and thus block hash)
// is consistent throughout the database.
func patchState(datadir string, patches []Patch) error {
	chaindata := filepath.Join(datadir, "geth", "chaindata")

	// ----------------------------------------------------------------
	// 1. Open database
	// ----------------------------------------------------------------
	fmt.Printf("\nopening database: %s\n", chaindata)
	kvStore, err := pebble.New(chaindata, 256, 256, "", false)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	db := rawdb.NewDatabase(kvStore)
	defer db.Close()

	// ----------------------------------------------------------------
	// 2. Read head block
	// ----------------------------------------------------------------
	oldHash := rawdb.ReadHeadBlockHash(db)
	if oldHash == (common.Hash{}) {
		return fmt.Errorf("head block hash not found in database")
	}
	num, ok := rawdb.ReadHeaderNumber(db, oldHash)
	if !ok {
		return fmt.Errorf("block number not found for hash %s", oldHash)
	}
	header := rawdb.ReadHeader(db, oldHash, num)
	if header == nil {
		return fmt.Errorf("header not found: hash=%s num=%d", oldHash, num)
	}
	fmt.Printf("head block: num=%d hash=%s root=%s\n", num, oldHash, header.Root)

	// ----------------------------------------------------------------
	// 3. Open state trie (auto-detect hash vs path scheme)
	// ----------------------------------------------------------------
	tdbConfig := triedb.Config{}
	scheme := rawdb.ReadStateScheme(db)
	switch scheme {
	case rawdb.PathScheme:
		fmt.Println("state scheme: path (PBSS)")
		tdbConfig.PathDB = pathdb.Defaults
	default:
		fmt.Println("state scheme: hash")
		tdbConfig.HashDB = hashdb.Defaults
	}

	tdb := triedb.NewDatabase(db, &tdbConfig)
	defer tdb.Close()

	sdb := state.NewDatabase(tdb, nil)
	stateDB, err := state.New(header.Root, sdb)
	if err != nil {
		return fmt.Errorf("open state at root %s: %w", header.Root, err)
	}

	// ----------------------------------------------------------------
	// 4. Apply storage patches
	// ----------------------------------------------------------------
	fmt.Println("\napplying patches:")
	for _, p := range patches {
		if !stateDB.Exist(p.Address) {
			fmt.Printf("  WARNING: account %s does not exist, will be created\n", p.Address)
		}
		prev := stateDB.GetState(p.Address, p.Slot)
		stateDB.SetState(p.Address, p.Slot, p.Value)
		fmt.Printf("  %s [%s]\n    %s → %s\n",
			p.Address, p.Slot, prev, p.Value)
	}

	// ----------------------------------------------------------------
	// 5. Commit state → new state root
	// ----------------------------------------------------------------
	newRoot, err := stateDB.Commit(num, false, true)
	if err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	// HashDB scheme requires Reference before Commit to mark the root reachable
	if err := tdb.Reference(newRoot, common.Hash{}); err != nil {
		return fmt.Errorf("reference trie root: %w", err)
	}
	if err := tdb.Commit(newRoot, false); err != nil {
		return fmt.Errorf("commit trie db: %w", err)
	}

	if newRoot == header.Root {
		fmt.Println("\nstate root unchanged — nothing to update")
		return nil
	}
	fmt.Printf("\nnew state root: %s\n", newRoot)

	// ----------------------------------------------------------------
	// 6. Clone header with new state root → new block hash
	// ----------------------------------------------------------------
	newHeader := types.CopyHeader(header)
	newHeader.Root = newRoot
	newHash := newHeader.Hash()
	fmt.Printf("new block hash: %s (was %s)\n", newHash, oldHash)

	// ----------------------------------------------------------------
	// 7. Re-key block data: old hash → new hash
	// ----------------------------------------------------------------
	fmt.Println("\nre-keying block data...")

	// Write header
	rawdb.WriteHeader(db, newHeader)

	// Body
	body := rawdb.ReadBody(db, oldHash, num)
	if body != nil {
		rawdb.WriteBody(db, newHash, num, body)
	}

	// ----------------------------------------------------------------
	// 8. Update canonical hash and head pointers
	// ----------------------------------------------------------------
	rawdb.WriteCanonicalHash(db, newHash, num)
	rawdb.WriteHeadBlockHash(db, newHash)
	rawdb.WriteHeadHeaderHash(db, newHash)

	// ----------------------------------------------------------------
	// 9. Delete old hash entries
	// ----------------------------------------------------------------
	rawdb.DeleteHeader(db, oldHash, num)
	rawdb.DeleteBody(db, oldHash, num)

	fmt.Println("block data re-keyed successfully")
	return nil
}
