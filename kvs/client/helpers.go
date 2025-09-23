package main

import "hash/fnv"

func dialHosts(servers HostList) []*Client {
	var clients []*Client
	for _, addr := range servers {
		clients = append(clients, Dial(addr))
	}
	return clients
}

func serverFromKey(key *string, servers []*Client) *Client {
	h := fnv.New32a()
	h.Write([]byte(*key))
	idx := int(h.Sum32()) % len(servers)
	return servers[idx]
}
