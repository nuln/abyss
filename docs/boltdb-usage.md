# BoltDB Usage

## Description
Instructions on interacting with the BoltDB database through the `boltdb.go` and `storage.go` abstractions in Abyss.

## Context
- `boltdb.go`: BoltDB implementation and store wrappers.
- `storage.go`: Higher-level file and storage metadata management.

## Instructions
1. **Transactions**:
   - Use `boltView(db, fn)` for read-only operations.
   - Use `boltUpdate(db, fn)` for read-write operations.
   - BoltDB supports only one concurrent writer but multiple concurrent readers.
2. **Buckets**:
   - Data is organized into buckets (e.g., `identity_users`, `storage_files`).
   - Use `tx.Bucket(name)` to access a bucket.
   - Use `tx.CreateBucketIfNotExists(name)` to ensure a bucket exists.
3. **Indexing**:
   - BoltDB is a KV store. For non-PK lookups (like email), use separate index buckets that map the secondary key to the Primary Key.
4. **Serialization**:
   - Use `boltMarshal(v)` (JSON) before storing values.
   - Use `boltUnmarshal(data, &v)` after retrieving values.
5. **IDs**:
   - Primary Keys are usually `uint64`. Use `boltNextID(bucket)` to generate the next sequence number.
   - Store these keys using `boltUint64Key(id)` to ensure correct byte-order sorting.
6. **Best Practices**:
   - Keep transactions short to avoid blocking writers.
   - Avoid nesting `boltUpdate` calls as it can lead to deadlocks.
