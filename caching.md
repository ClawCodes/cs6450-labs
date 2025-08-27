# Client-side Caching Strategy

- Set up tcp scanner on clients waiting for invalidation messages from the server
- Client cache with concurrency safe key-(value, invalidation channel) store
- Server cache with concurrency safe list of key-(subscribers tcp) store
- Not thinking about max cache size and removing cache elements at the moment

`client.Put(key, value)` written to server KV store -> `server.invalidateSubscribers(key)` *maybe return list of invalid cache entries on each put?*
`client.Get(key)` cached value ? (invalidation message ? `client.handleInvalidation(key)` (`client.fromServer(key)` -> put in cache) : get from cache) : (`client.fromServer(key)` -> put in cache + `client.subscribe(key)`)