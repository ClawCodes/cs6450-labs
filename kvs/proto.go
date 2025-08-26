package kvs

type PutRequest struct {
	Key   string
	Value string
}

type PutResponse struct {
}

type GetRequest struct {
	Key string
}

type GetResponse struct {
	Value string
}

type BatchGetRequest struct {
	Keys []string
}

type BatchGetResponse struct {
	Values []string
}

type BatchPutRequest struct {
	Keys   []string
	Values []string
}

type BatchPutResponse struct {
}
