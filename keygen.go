package main

import (
	"crypto/rand"
	"flag"
	"io"
	"log"
	"os"
	"time"

	serf "github.com/hashicorp/serf/client"
)

const (
	tick = 15 * time.Minute

	ahead  = 2
	behind = 26 * int(time.Hour/tick)

	total = ahead + 1 + behind

	nameLen = 16
	encLen  = 16
	hmacLen = 16
	keySize = nameLen + encLen + hmacLen
)

var keys = make([][keySize]byte, 0, total)

func main() {
	conf := &serf.Config{}

	flag.StringVar(&conf.Addr, "addr", "127.0.0.1:7373", "the address to connect to")
	flag.StringVar(&conf.AuthKey, "auth", "", "the RPC auth key")
	flag.DurationVar(&conf.Timeout, "timeout", 0, "the RPC timeout")

	transName := flag.String("transaction-log", "trans.log", "the transaction log")

	flag.Parse()

	var trans *log.Logger

	if len(*transName) != 0 {
		transFile, err := os.OpenFile(*transName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			panic(err)
		}

		defer transFile.Close()

		trans = log.New(io.MultiWriter(os.Stderr, transFile), "", log.LstdFlags|log.LUTC)
	} else {
		trans = log.New(os.Stderr, "", log.LstdFlags|log.LUTC)
	}

	rpc, err := serf.ClientFromConfig(conf)
	if err != nil {
		panic(err)
	}

	// TODO: rpc.Join([]string{}, false)

	log.Printf("storing %d keys ahead, %d behind; using each key for %s", ahead, behind, tick)

	// TODO: retrieve old keys from network...

	for i := 0; i < ahead+1; i++ {
		var key [keySize]byte

		if _, err = rand.Read(key[:]); err != nil {
			panic(err)
		}

		trans.Printf("install-key %x", key[:nameLen:nameLen])
		if err = rpc.UserEvent("install-key", key[:], false); err != nil {
			panic(err)
		}

		// zero key material, leaving name
		for i := nameLen; i < keySize; i++ {
			key[i] = 0
		}

		keys = append([][keySize]byte{key}, keys...)
	}

	// TODO: wait

	trans.Printf("set-default-key %x", keys[ahead][:nameLen:nameLen])
	if err = rpc.UserEvent("set-default-key", keys[ahead][:nameLen:nameLen], false); err != nil {
		panic(err)
	}

	for range time.Tick(tick) {
		var key [keySize]byte

		if _, err = rand.Read(key[:]); err != nil {
			panic(err)
		}

		trans.Printf("install-key %x", key[:nameLen:nameLen])
		if err = rpc.UserEvent("install-key", key[:], false); err != nil {
			panic(err)
		}

		// zero key material, leaving name
		for i := nameLen; i < keySize; i++ {
			key[i] = 0
		}

		if len(keys) == total {
			trans.Printf("remove-key %x", keys[total-1][:nameLen:nameLen])
			if err = rpc.UserEvent("remove-key", keys[total-1][:nameLen:nameLen], false); err != nil {
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

		trans.Printf("set-default-key %x", keys[ahead][:nameLen:nameLen])
		if err = rpc.UserEvent("set-default-key", keys[ahead][:nameLen:nameLen], false); err != nil {
			panic(err)
		}
	}
}
