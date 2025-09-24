// put this file in kvs/client
// stand up a server first with:
// $ make run-server
// then
// $ go test ./kvs/client/
// keep in mind this modifies server state, so
// it's a good idea to restart the server in between test runs.
package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var hosts = []string{"localhost:8080"}

func TestNop(t *testing.T) {
	clients := dialHosts(hosts)
	txn := Txn{}
	txn.Begin(clients)
	err := txn.Commit()
	assert.Nil(t, err)
}

func putTx(clients []*Client, k string, v string) error {
	txn := Txn{}
	txn.Begin(clients)
	err := txn.Put(k, v)
	if err != nil {
		return err
	}
	return txn.Commit()
}

func getTx(clients []*Client, k string) (string, error) {
	txn := Txn{}
	txn.Begin(clients)
	value, err := txn.Get(k)
	if err != nil {
		return "", err
	}
	err = txn.Commit()
	return value, err
}

func TestPutGet(t *testing.T) {
	clients := dialHosts(hosts)

	// tx0
	txn := Txn{}
	txn.Begin(clients)

	err := txn.Put("test", "value")
	assert.Nil(t, err)

	got, err := txn.Get("test")
	assert.Nil(t, err)
	assert.Equal(t, "value", got)

	err = txn.Commit()
	assert.Nil(t, err)

	// tx1 - read committed value
	got, err = getTx(clients, "test")
	assert.Nil(t, err)
	assert.Equal(t, "value", got)

	// Test multiple put/get cycles
	for i := 0; i < 1024; i++ {
		is := fmt.Sprintf("%v", i)
		err := putTx(clients, is, is)
		assert.Nil(t, err)

		r, err := getTx(clients, is)
		assert.Nil(t, err)
		assert.Equal(t, is, r)
	}

	// Test batch puts then batch gets
	for i := 0; i < 1024; i++ {
		is := fmt.Sprintf("%v", i)
		err := putTx(clients, is, is)
		assert.Nil(t, err)
	}

	for i := 0; i < 1024; i++ {
		is := fmt.Sprintf("%v", i)
		r, err := getTx(clients, is)
		assert.Nil(t, err)
		assert.Equal(t, is, r)
	}
}

func TestWWConflict(t *testing.T) {
	clients := dialHosts(hosts)

	txn1 := Txn{}
	txn2 := Txn{}

	txn1.Begin(clients)
	txn2.Begin(clients)

	err := txn1.Put("c1", "c1")
	assert.Nil(t, err)

	err = txn2.Put("c2", "c2")
	assert.Nil(t, err)

	err = txn1.Commit()
	assert.Nil(t, err)

	err = txn2.Commit()
	assert.Nil(t, err)

	// Test write-write conflict on same key
	txn1 = Txn{}
	txn2 = Txn{}

	txn1.Begin(clients)
	txn2.Begin(clients)

	err = txn1.Put("conflict_key", "c1")
	assert.Nil(t, err)

	err = txn2.Put("conflict_key", "c2")
	assert.NotNil(t, err, "Second Put on same key should fail")

	err = txn1.Commit()
	assert.Nil(t, err)

	// txn2 should already be aborted, commit should work but transaction was aborted
	err = txn2.Commit()
	assert.Nil(t, err) // Commit doesn't fail, but transaction was already aborted
}

func TestRWConflict(t *testing.T) {
	clients := dialHosts(hosts)

	txn1 := Txn{}
	txn2 := Txn{}

	txn1.Begin(clients)
	txn2.Begin(clients)

	err := txn1.Put("rw1", "c1")
	assert.Nil(t, err)

	_, err = txn2.Get("rw2")
	assert.Nil(t, err) // Reading different key should work

	err = txn1.Commit()
	assert.Nil(t, err)

	err = txn2.Commit()
	assert.Nil(t, err)

	// Test read-write conflict
	txn1 = Txn{}
	txn2 = Txn{}

	txn1.Begin(clients)
	txn2.Begin(clients)

	got, err := txn1.Get("rw1")
	assert.Nil(t, err)
	assert.Equal(t, "c1", got)

	err = txn2.Put("rw1", "c2")
	assert.NotNil(t, err, "Write after read should cause conflict")

	err = txn1.Commit()
	assert.Nil(t, err)

	// txn2 was aborted
	err = txn2.Commit()
	assert.Nil(t, err)

	// Test write-read conflict
	txn1 = Txn{}
	txn2 = Txn{}

	txn1.Begin(clients)
	txn2.Begin(clients)

	err = txn1.Put("rw1", "c3")
	assert.Nil(t, err)

	_, err = txn2.Get("rw1")
	assert.NotNil(t, err, "Read after write should cause conflict")

	err = txn1.Commit()
	assert.Nil(t, err)

	// txn2 was aborted
	err = txn2.Commit()
	assert.Nil(t, err)
}
