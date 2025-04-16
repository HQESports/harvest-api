// Primary time-based index for range queries
db.matches.createIndex({ "created_at": 1 })

// Compound indexes for metrics queries
db.matches.createIndex({ "processed": 1, "created_at": 1 })
db.matches.createIndex({ "map_name": 1, "processed": 1 })
db.matches.createIndex({ "match_type": 1, "processed": 1 })

// Partial indexes for optimizing queries on processed matches
db.matches.createIndex(
  { "created_at": 1, "map_name": 1 },
  { partialFilterExpression: { processed: true } }
)

db.matches.createIndex(
  { "created_at": 1, "match_type": 1 },
  { partialFilterExpression: { processed: true } }
)

// Index for match_id lookups
db.matches.createIndex({ "match_id": 1 }, { unique: true })