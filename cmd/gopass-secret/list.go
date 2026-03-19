package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/godbus/dbus/v5"

	dbustypes "github.com/nikicat/gopass-secret-service/internal/dbus"
)

const defaultMaxWidth = 30

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	maxWidth := fs.Int("max-width", defaultMaxWidth, "Max attribute value width (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	var filterCollection string
	if fs.NArg() > 0 {
		filterCollection = fs.Arg(0)
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("Failed to connect to session bus: %v", err)
	}
	defer conn.Close()

	svc := conn.Object(dbustypes.ServiceName, dbustypes.ServicePath)

	// Get collection paths from the Collections property
	variant, err := svc.GetProperty(dbustypes.SecretServiceInterface + ".Collections")
	if err != nil {
		log.Fatalf("Failed to get collections: %v", err)
	}

	collPaths, ok := variant.Value().([]dbus.ObjectPath)
	if !ok {
		log.Fatalf("Unexpected Collections property type: %T", variant.Value())
	}

	type row struct {
		collection string
		id         string
		label      string
		attrs      map[string]string
	}

	var rows []row
	attrKeys := make(map[string]bool)

	for _, collPath := range collPaths {
		collName, err := dbustypes.ParseCollectionPath(collPath)
		if err != nil {
			log.Printf("Warning: invalid collection path %s: %v", collPath, err)
			continue
		}

		if filterCollection != "" && collName != filterCollection {
			continue
		}

		collObj := conn.Object(dbustypes.ServiceName, collPath)

		// Get item paths from the collection's Items property
		itemsVariant, err := collObj.GetProperty(dbustypes.CollectionInterface + ".Items")
		if err != nil {
			log.Printf("Warning: failed to get items for %s: %v", collName, err)
			continue
		}

		itemPaths, ok := itemsVariant.Value().([]dbus.ObjectPath)
		if !ok {
			log.Printf("Warning: unexpected Items property type for %s: %T", collName, itemsVariant.Value())
			continue
		}

		for _, itemPath := range itemPaths {
			_, itemID, err := dbustypes.ParseItemPath(itemPath)
			if err != nil {
				log.Printf("Warning: invalid item path %s: %v", itemPath, err)
				continue
			}

			itemObj := conn.Object(dbustypes.ServiceName, itemPath)

			// Get all item properties at once
			var allProps map[string]dbus.Variant
			err = itemObj.Call("org.freedesktop.DBus.Properties.GetAll", 0, dbustypes.ItemInterface).Store(&allProps)
			if err != nil {
				log.Printf("Warning: failed to get properties for %s: %v", itemPath, err)
				continue
			}

			label := ""
			if v, ok := allProps["Label"]; ok {
				if s, ok := v.Value().(string); ok {
					label = s
				}
			}

			attrs := map[string]string{}
			if v, ok := allProps["Attributes"]; ok {
				if a, ok := v.Value().(map[string]string); ok {
					attrs = a
				}
			}

			for k := range attrs {
				attrKeys[k] = true
			}

			rows = append(rows, row{
				collection: collName,
				id:         itemID,
				label:      label,
				attrs:      attrs,
			})
		}
	}

	if filterCollection != "" && len(rows) == 0 {
		// Check if the collection existed at all
		found := false
		for _, collPath := range collPaths {
			name, _ := dbustypes.ParseCollectionPath(collPath)
			if name == filterCollection {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "Collection not found: %s\n", filterCollection)
			os.Exit(1)
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
