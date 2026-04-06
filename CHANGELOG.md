# CHANGELOG

## v0.3.0

Adds a new function `DeleteAttribute` that allows a user to delete a single attribute from an entity.

## v0.2.1

Something went wrong releasing v0.2.0, and pkg.go.dev has the wrong commit for that tag.

## v0.2.0

1. Fixes a bug in how string lists are stored. Prior to this, string lists were stored in DynamoDB as `SS` or String Set. As such, they were unordered and not good for storing lists. Starting with this version, string lists are stored as `L` with elements stored as `S`.
2. `CreateEntity` now expects a list of attributes (may be `nil`)

## v0.1.0

Initial release of this package. Entities must have a type, ID, label, and parent ID. Entities may have attributes. Attributes may be strings or lists of strings.
