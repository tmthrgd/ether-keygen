package main

import (
	"crypto/rand"
	"log"
	"time"

	serf "github.com/hashicorp/serf/client"
)

const (
	// tick = 1 * time.Hour
	tick = 5 * time.Second

	ahead  = 2
	behind = 26

	total = ahead + 1 + behind

	nameLen = 16
	encLen  = 16
	hmacLen = 16
	keySize = nameLen + encLen + hmacLen
)

var keys = make([][keySize]byte, 0, total)

func main() {
	conf := &serf.Config{Addr: "127.0.0.1:7373"}

	// TODO: conf.Addr
	// TODO: conf.AuthKey
	// TODO: conf.Timeout

	rpc, err := serf.ClientFromConfig(conf)
	if err != nil {
		panic(err)
	}

	// TODO: rpc.Join([]string{}, false)

	// TODO: retrieve old keys from network...

	for i := 0; i < ahead+1; i++ {
		var key [keySize]byte

		if _, err = rand.Read(key[:]); err != nil {
			panic(err)
		}

		log.Printf("install-key: %x", key[:nameLen])
		if err = rpc.UserEvent("install-key", key[:], false); err != nil {
			panic(err)
		}

		keys = append([][keySize]byte{key}, keys...)
	}

	// TODO: wait

	log.Printf("use-key: %x", keys[ahead][:nameLen])
	if err = rpc.UserEvent("use-key", keys[ahead][:nameLen], false); err != nil {
		panic(err)
	}

	for range time.Tick(tick) {
		var key [keySize]byte

		if _, err = rand.Read(key[:]); err != nil {
			panic(err)
		}

		log.Printf("install-key: %x", key[:nameLen])
		if err = rpc.UserEvent("install-key", key[:], false); err != nil {
			panic(err)
		}

		if len(keys) == total {
			log.Printf("remove-key: %x", keys[total-1][:nameLen])
			if err = rpc.UserEvent("remove-key", keys[total-1][:nameLen], false); err != nil {
				panic(err)
			}

			// zero old key
			for i := 0; i < keySize; i++ {
				keys[total-1][i] = 0
			}

			keys = append([][keySize]byte{key}, keys[:total-1]...)
		} else {
			keys = append([][keySize]byte{key}, keys...)
		}

		log.Printf("use-key: %x", keys[ahead][:nameLen])
		if err = rpc.UserEvent("use-key", keys[ahead][:nameLen], false); err != nil {
			panic(err)
		}
	}
}
