package kvs

type PutRequest struct {
	Key   string
	Value string
}

type PutResponse struct {
}

type GetRequest struct {
	Key        string
	ClientAddr string
}

type GetResponse struct {
	Value string
}

type UpdateRequest struct {
	Key   string
	Value string
}

type UpdateResponse struct {
}
