package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/nikicat/gopass-secret-service/internal/store"
)

const defaultMaxWidth = 30

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var flags commonFlags
	addCommonFlags(fs, &flags)
	maxWidth := fs.Int("max-width", defaultMaxWidth, "Max attribute value width (0 = unlimited)")
	fs.Parse(args)

	cfg, err := flags.loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()
	s, err := store.NewGopassStore(ctx, cfg.Prefix)
	if err != nil {
		log.Fatalf("Failed to open gopass store: %v", err)
	}
	defer s.Close(ctx)

	// Get collections, optionally filtered by positional arg
	var filterCollection string
	if fs.NArg() > 0 {
		filterCollection = fs.Arg(0)
	}

	collections, err := s.Collections(ctx)
	if err != nil {
		log.Fatalf("Failed to list collections: %v", err)
	}

	if filterCollection != "" {
		found := false
		for _, c := range collections {
			if c == filterCollection {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "Collection not found: %s\n", filterCollection)
			os.Exit(1)
		}
		collections = []string{filterCollection}
	}

	// Collect all items and discover unique attribute keys
	type row struct {
		collection string
		id         string
		label      string
		attrs      map[string]string
	}

	var rows []row
	attrKeys := make(map[string]bool)

	for _, coll := range collections {
		items, err := s.Items(ctx, coll)
		if err != nil {
			log.Printf("Warning: failed to list items in %s: %v", coll, err)
			continue
		}

		for _, id := range items {
			item, err := s.GetItem(ctx, coll, id)
			if err != nil {
				log.Printf("Warning: failed to get item %s/%s: %v", coll, id, err)
				continue
			}

			for k := range item.Attributes {
				attrKeys[k] = true
			}

			rows = append(rows, row{
				collection: coll,
				id:         id,
				label:      item.Label,
				attrs:      item.Attributes,
			})
		}
	}

	// Sort attribute keys
	sortedKeys := make([]string, 0, len(attrKeys))
	for k := range attrKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintf(w, "COLLECTION\tID\tLABEL")
	for _, k := range sortedKeys {
		fmt.Fprintf(w, "\t%s", k)
	}
	fmt.Fprintln(w)

	// Rows
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s", r.collection, r.id, truncate(r.label, *maxWidth))
		for _, k := range sortedKeys {
			fmt.Fprintf(w, "\t%s", truncate(r.attrs[k], *maxWidth))
		}
		fmt.Fprintln(w)
	}

	w.Flush()
}

func truncate(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}
