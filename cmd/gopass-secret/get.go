package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"

	dbustypes "github.com/nikicat/gopass-secret-service/internal/dbus"
	"github.com/nikicat/gopass-secret-service/internal/schema"
)

func runGet(args []string) {
	if len(args) == 0 {
		printGetUsage()
		os.Exit(1)
	}

	typeName := args[0]
	if typeName == "-h" || typeName == "--help" {
		printGetUsage()
		return
	}
	st, ok := schema.Get(typeName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown secret type: %s\n\n", typeName)
		printGetUsage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("get "+typeName, flag.ExitOnError)

	// Register type-specific attribute flags (all optional for search)
	attrFlags := make(map[string]*string)
	for _, attr := range st.Attributes {
		attrFlags[attr.Name] = fs.String(attr.Name, "", attr.Help)
	}

	mustParse(fs, args[1:])

	// Build attributes map
	attrs := map[string]string{
		"xdg:schema": st.Schema,
	}

	for name, val := range attrFlags {
		if *val != "" {
			attrs[name] = *val
		}
	}

	// For generic type, collect remaining positional args as key=value
	if typeName == "generic" {
		for _, arg := range fs.Args() {
			k, v, ok := strings.Cut(arg, "=")
			if !ok {
				log.Fatalf("Invalid attribute (expected key=value): %s", arg)
			}
			if k == "xdg:schema" {
				log.Fatal("Cannot override xdg:schema via positional args")
			}
			attrs[k] = v
		}
	}

	secret, err := lookupSecret(attrs)
	if err != nil {
		log.Fatal(err)
	}

	os.Stdout.Write(secret)
}

func lookupSecret(attrs map[string]string) ([]byte, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connecting to session bus: %w", err)
	}
	defer conn.Close()

	svc := conn.Object(dbustypes.ServiceName, dbustypes.ServicePath)

	// Open session
	var sessionOutput dbus.Variant
	var sessionPath dbus.ObjectPath
	err = svc.Call(
		dbustypes.SecretServiceInterface+".OpenSession",
		0,
		dbustypes.AlgorithmPlain,
		dbus.MakeVariant(""),
	).Store(&sessionOutput, &sessionPath)
	if err != nil {
		return nil, fmt.Errorf("opening session: %w", err)
	}
	defer conn.Object(dbustypes.ServiceName, sessionPath).Call(
		dbustypes.SessionInterface+".Close", 0,
	)

	// SearchItems returns (unlocked, locked)
	var unlocked, locked []dbus.ObjectPath
	err = svc.Call(
		dbustypes.SecretServiceInterface+".SearchItems",
		0,
		attrs,
	).Store(&unlocked, &locked)
	if err != nil {
		return nil, fmt.Errorf("searching items: %w", err)
	}

	if len(locked) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d locked item(s) skipped\n", len(locked))
	}

	if len(unlocked) == 0 {
		return nil, fmt.Errorf("no matching secrets found")
	}

	// GetSecrets for the first unlocked item
	var secrets map[dbus.ObjectPath]dbustypes.Secret
	err = svc.Call(
		dbustypes.SecretServiceInterface+".GetSecrets",
		0,
		unlocked[:1],
		sessionPath,
	).Store(&secrets)
	if err != nil {
		return nil, fmt.Errorf("getting secret: %w", err)
	}

	for _, s := range secrets {
		return s.Value, nil
	}

	return nil, fmt.Errorf("no matching secrets found")
}

func printGetUsage() {
	fmt.Fprint(os.Stderr, `Usage: gopass-secret get <type> [flags...] [key=value...]

Searches for a secret by type and optional attributes, prints its value to stdout.

Secret types:
`)
	for _, name := range schema.TypeNames() {
		st := schema.Types[name]
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", name, st.Help)
		for _, attr := range st.Attributes {
			fmt.Fprintf(os.Stderr, "    --%-10s %s\n", attr.Name, attr.Help)
		}
	}
	fmt.Fprint(os.Stderr, `
The generic type accepts arbitrary key=value positional arguments.
`)
}
