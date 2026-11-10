---
status: todo
tags: [unsmell]
---
# Collection.Export and ExportAtPath repeat the same exports and XML

## Goal
Collection.Export and Collection.ExportAtPath bind the same three interfaces with the same rollback and the same 47-line introspection XML; Export only adds logging and ExportAllItems. The changeset had to edit four identical lines in each copy (collection.go:146-155 and 232-238, 208 and 291).

Where: internal/service/collection.go:141-214, internal/service/collection.go:228-296. Found by the /unsmell run of 2026-11-10 (duplication).

## Next
Export becomes ExportAtPath(c.path) followed by c.svc.items.ExportAllItems(c.name). The XML becomes one package-level const, and ExportAtPath ends with return export(…) instead of if err … return err; return nil.
