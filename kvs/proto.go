package kvs

type PutRequest struct {
	Key   string
	Value string
	Txid uint64
}

type PutResponse struct {
	Error bool
}

type GetRequest struct {
	Key string
	Txid uint64
}

type GetResponse struct {
	Value string
	Error bool
}
