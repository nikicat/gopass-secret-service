package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"
	dbustypes "github.com/nikicat/gopass-secret-service/internal/dbus"
	"github.com/nikicat/gopass-secret-service/internal/schema"
	"golang.org/x/term"
)

func runAdd(args []string) {
	if len(args) == 0 {
		printAddUsage()
		os.Exit(1)
	}

	typeName := args[0]
	st, ok := schema.Get(typeName)
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown secret type: %s\n\n", typeName)
		printAddUsage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet("add "+typeName, flag.ExitOnError)
	label := fs.String("label", "", "Label for the secret (required)")
	collection := fs.String("collection", "default", "Collection name or alias")

	// Register type-specific attribute flags
	attrFlags := make(map[string]*string)
	for _, attr := range st.Attributes {
		hint := attr.Help
		if attr.Required {
			hint += " (required)"
		}
		attrFlags[attr.Name] = fs.String(attr.Name, "", hint)
	}

	fs.Parse(args[1:])

	if *label == "" {
		log.Fatal("--label is required")
	}

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

	// Validate required attributes
	if err := schema.Validate(st, attrs); err != nil {
		log.Fatal(err)
	}

	// Read secret value
	secret, err := readSecret()
	if err != nil {
		log.Fatalf("Failed to read secret: %v", err)
	}
	if len(secret) == 0 {
		log.Fatal("Secret value cannot be empty")
	}

	// D-Bus interaction
	itemPath, err := createItem(*collection, *label, attrs, secret)
	if err != nil {
		log.Fatalf("Failed to create secret: %v", err)
	}

	fmt.Println(itemPath)
}

func readSecret() ([]byte, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Secret: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr) // newline after hidden input
		return b, err
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	// Trim single trailing newline (common with echo "x" | ...)
	b = []byte(strings.TrimRight(string(b), "\n"))
	return b, nil
}

func createItem(collection, label string, attrs map[string]string, secret []byte) (string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", fmt.Errorf("connecting to session bus: %w", err)
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
		return "", fmt.Errorf("opening session: %w", err)
	}
	defer conn.Object(dbustypes.ServiceName, sessionPath).Call(
		dbustypes.SessionInterface+".Close", 0,
	)

	// Collection path
	collPath := dbus.ObjectPath(dbustypes.AliasBasePath + "/" + collection)

	// Properties
	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label":      dbus.MakeVariant(label),
		"org.freedesktop.Secret.Item.Attributes":  dbus.MakeVariant(attrs),
	}

	// Secret struct: (session, params, value, content-type)
	secretStruct := dbustypes.Secret{
		Session:     sessionPath,
		Parameters:  []byte{},
		Value:       secret,
		ContentType: "text/plain",
	}

	// CreateItem(properties, secret, replace)
	collObj := conn.Object(dbustypes.ServiceName, collPath)
	var itemPath dbus.ObjectPath
	var promptPath dbus.ObjectPath
	err = collObj.Call(
		dbustypes.CollectionInterface+".CreateItem",
		0,
		props,
		secretStruct,
		true,
	).Store(&itemPath, &promptPath)
	if err != nil {
		return "", fmt.Errorf("creating item: %w", err)
	}

	return string(itemPath), nil
}

func printAddUsage() {
	fmt.Fprint(os.Stderr, `Usage: gopass-secret add <type> --label LABEL [--collection NAME] [flags...] [key=value...]

Secret types:
`)
	for _, name := range schema.TypeNames() {
		st := schema.Types[name]
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", name, st.Help)
		for _, attr := range st.Attributes {
			req := ""
			if attr.Required {
				req = " (required)"
			}
			fmt.Fprintf(os.Stderr, "    --%-10s %s%s\n", attr.Name, attr.Help, req)
		}
	}
	fmt.Fprint(os.Stderr, `
The generic type accepts arbitrary key=value positional arguments.

The secret value is read from stdin. If stdin is a terminal, you will be
prompted with hidden input.
`)
}
