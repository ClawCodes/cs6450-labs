package kvs

type PutRequest struct {
	Key   string
	Value string
}

type PutResponse struct {
}

type GetRequest struct {
	Key   string
	Value string
}

type GetResponse struct {
	Key   string
	Value string
}

type RegisterCacheRequest struct {
	Key      string
	ClientID string
}

type RegisterCacheResponse struct {
}

type InvalidationRequest struct {
	Key string
}

type InvalidationResponse struct {
}
