# Architecture

This document describes the architecture of gopass-secret-service.

## Overview

gopass-secret-service implements the [freedesktop.org Secret Service specification](https://specifications.freedesktop.org/secret-service/latest/) as a D-Bus service, using GoPass as the backend for secret storage.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Desktop Applications                         │
│         (Firefox, Chrome, GNOME apps, secret-tool, etc.)        │
└─────────────────────────────────────────────────────────────────┘
                                │
                                │ D-Bus Session Bus
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    gopass-secret-service                         │
│  ┌─────────────┐  ┌─────────────┐  ┌──────────────────────────┐ │
│  │   Service   │  │ Collections │  │         Items            │ │
│  │   (/org/    │──│ (/org/.../  │──│  (/org/.../collection/   │ │
│  │  freedesktop│  │ collection/ │  │         name/id)         │ │
│  │  /secrets)  │  │    name)    │  │                          │ │
│  └─────────────┘  └─────────────┘  └──────────────────────────┘ │
│         │                │                      │               │
│         └────────────────┼──────────────────────┘               │
│                          ▼                                       │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                      Store Layer                             ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────────┐  ││
│  │  │ GopassStore  │  │    Mapper    │  │   ItemData/       │  ││
│  │  │ (CLI wrapper)│  │ (path conv.) │  │  CollectionData   │  ││
│  │  └──────────────┘  └──────────────┘  └───────────────────┘  ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                │
                                │ CLI invocation
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                         gopass CLI                               │
│                  (show, insert, ls, rm, etc.)                   │
└─────────────────────────────────────────────────────────────────┘
                                │
                                │ File I/O + GPG
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                    ~/.password-store/                            │
│                     (GPG-encrypted files)                        │
└─────────────────────────────────────────────────────────────────┘
```

## Components

### D-Bus Layer (`internal/dbus/`)

- **types.go**: D-Bus type definitions (Secret struct, interface names, paths)
- **paths.go**: Utilities for constructing and parsing D-Bus object paths

### Service Layer (`internal/service/`)

- **service.go**: Main `org.freedesktop.Secret.Service` implementation
  - Name acquisition on the session bus
  - OpenSession, CreateCollection, SearchItems, Lock/Unlock, GetSecrets
  - ReadAlias/SetAlias for collection aliases
  - Signal emission (CollectionCreated, etc.)

- **collection.go**: `org.freedesktop.Secret.Collection` implementation
  - CreateItem, SearchItems, Delete
  - Property management (Items, Label, Locked, Created, Modified)
  - CollectionManager for lifecycle management

- **item.go**: `org.freedesktop.Secret.Item` implementation
  - GetSecret, SetSecret, Delete
  - Property management (Attributes, Label, Locked, Created, Modified)
  - ItemManager for lifecycle management

- **session.go**: `org.freedesktop.Secret.Session` implementation
  - Session lifecycle management
  - Encryption/decryption wrapper

- **prompt.go**: `org.freedesktop.Secret.Prompt` implementation
  - Prompt lifecycle for operations requiring user interaction
  - Completed signal emission

- **errors.go**: D-Bus error definitions per the Secret Service spec

### Crypto Layer (`internal/crypto/`)

- **crypto.go**: Session interface and factory
- **plain.go**: "plain" algorithm implementation (no encryption for local D-Bus transfer)

The crypto layer is designed to be extensible. While only "plain" is currently implemented, the interface allows adding encrypted transports (e.g., `dh-ietf1024-sha256-aes128-cbc-pkcs7`).

### Store Layer (`internal/store/`)

- **store.go**: Store interface defining all operations
- **gopass.go**: GoPass CLI wrapper implementation
- **mapper.go**: Path mapping between D-Bus paths and GoPass paths

### Configuration (`internal/config/`)

- **config.go**: CLI flag parsing, environment variables, config file loading

## D-Bus Interface Mapping

| D-Bus Interface | Object Path | Implementation |
|-----------------|-------------|----------------|
| org.freedesktop.Secret.Service | /org/freedesktop/secrets | service.Service |
| org.freedesktop.Secret.Collection | /org/freedesktop/secrets/collection/{name} | service.Collection |
| org.freedesktop.Secret.Item | /org/freedesktop/secrets/collection/{name}/{id} | service.Item |
| org.freedesktop.Secret.Session | /org/freedesktop/secrets/session/{id} | service.Session |
| org.freedesktop.Secret.Prompt | /org/freedesktop/secrets/prompt/{id} | service.Prompt |

## Data Flow

### Storing a Secret

1. Application calls `Collection.CreateItem()` with properties and secret
2. Service validates the session and decrypts the secret (if encrypted transport)
3. Store layer generates a UUID and formats the item
4. GoPass CLI is invoked to insert the secret
5. Item is exported as a D-Bus object
6. ItemCreated signal is emitted

### Retrieving a Secret

1. Application calls `Item.GetSecret(session)`
2. Service validates the session
3. Store layer reads the item from GoPass
4. Secret is encrypted using the session's crypto (no-op for plain)
5. Secret struct is returned to the application

### Searching for Secrets

1. Application calls `Service.SearchItems(attributes)`
2. Store layer iterates through all collections and items
3. Items matching all specified attributes are returned
4. Results are split into locked/unlocked lists

## GoPass Integration

GoPass is invoked via CLI rather than as a library because:
1. The GoPass API is marked "DO NOT USE" and is unstable
2. CLI provides a stable interface
3. Proper GPG agent integration happens automatically

Commands used:
- `gopass ls --flat <prefix>` - List entries
- `gopass show -n <path>` - Read an entry (without newline)
- `gopass insert -f <path>` - Create/update an entry
- `gopass rm -f <path>` - Delete an entry
- `gopass rm -rf <path>` - Delete a directory (collection)

## Security Considerations

1. **Transport Security**: The "plain" algorithm is used for D-Bus communication. This is secure because D-Bus session bus communication is local and protected by UNIX socket permissions.

2. **Storage Security**: All secrets are stored encrypted using GPG via GoPass. The GPG key passphrase may be cached by gpg-agent.

3. **Lock State**: Lock state is tracked in-memory. When a collection is "locked", the underlying GPG-encrypted data remains accessible to processes with the correct GPG key.

4. **No Secret Logging**: Debug logging never logs secret values, only metadata.

## Extending

### Adding Encrypted Transport

1. Create a new file in `internal/crypto/` implementing the `Session` interface
2. Add the algorithm to `crypto.NewSession()` factory
3. Handle key exchange in `OpenSession`

### Adding a New Store Backend

1. Create a new file in `internal/store/` implementing the `Store` interface
2. Add a factory or configuration option to select the backend
3. Update `service.New()` to use the new store
