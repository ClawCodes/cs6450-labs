package kvs

type PutRequest struct {
	Key   string
	Value string
	Txid  uint64
}

type PutResponse struct {
}

type GetRequest struct {
	Key  string
	Txid uint64
}

type GetResponse struct {
	Value string
}

type CommitRequest struct {
	Txid uint64
	Lead bool
}

type CommitResponse struct {
}

type AbortRequest struct {
	Txid uint64
}

type AbortResponse struct {
}
