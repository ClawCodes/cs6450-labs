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


type Operation struct {
	OpType string // "GET" or "PUT"
	Key    string
	Value  string // Empty for GET operations
}

type BatchOpRequest struct {
	Operations []Operation
}

type BatchOpResponse struct {
	Results []string // Values for GET operations, empty for PUT
}
