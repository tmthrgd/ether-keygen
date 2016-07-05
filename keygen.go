package main

import (
	"crypto/rand"
	"flag"
	"log"
	"sync"
	"time"

	msgpack "github.com/hashicorp/go-msgpack/codec"
	serf "github.com/hashicorp/serf/client"
)

const (
	nameLen = 16

	installKeyEvent    = "install-key"
	removeKeyEvent     = "remove-key"
	setDefaultKeyEvent = "set-default-key"
	wipeKeysEvent      = "wipe-keys"

	retrieveKeysQuery = "retrieve-keys"
)

var keys [][]byte
var keysMut sync.RWMutex

func main() {
	conf := &serf.Config{}

	flag.StringVar(&conf.Addr, "addr", "127.0.0.1:7373", "the address to connect to")
	flag.StringVar(&conf.AuthKey, "auth", "", "the RPC auth key")
	flag.DurationVar(&conf.Timeout, "timeout", 0, "the RPC timeout")

	var eventKeyPrefix string
	flag.StringVar(&eventKeyPrefix, "prefix", "ether:", "the serf event prefix")

	var tick time.Duration
	flag.DurationVar(&tick, "tick", 15*time.Minute, "the time each key should be used")

	var ahead int
	flag.IntVar(&ahead, "ahead", 2, "the number of keys to create ahead of time")

	var behind int
	flag.IntVar(&behind, "behind", 26*int(time.Hour/(15*time.Minute)), "the number of keys to keep behind")

	var size int
	flag.IntVar(&size, "size", 256, "the bit length of the key to generate")

	flag.Parse()

	total := ahead + 1 + behind
	keySize := nameLen + size/8

	rpc, err := serf.ClientFromConfig(conf)
	if err != nil {
		panic(err)
	}

	log.Printf("storing %d %d-bit keys ahead (%s), %d behind (%s); using each key for %s", ahead, size, time.Duration(ahead)*tick, behind, time.Duration(behind)*tick, tick)

	queryCh := make(chan map[string]interface{})

	go func() {
		var buf []byte

		for req := range queryCh {
			if req["Name"] != eventKeyPrefix+retrieveKeysQuery {
				continue
			}

			keysMut.RLock()
			log.Printf("%s%s: %d keys", eventKeyPrefix, retrieveKeysQuery, len(keys))

			enc := msgpack.NewEncoderBytes(&buf, &msgpack.MsgpackHandle{RawToString: true, WriteExt: true})

			var resp struct {
				Default []byte
				Keys    [][]byte
			}

			if len(keys) >= ahead {
				resp.Default = keys[ahead][:nameLen:nameLen]
			}

			resp.Keys = keys

			if err := enc.Encode(resp); err != nil {
				panic(err)
			}

			id, ok := req["ID"].(uint64)

			if !ok {
				id = (uint64)(req["ID"].(int64))
			}

			if err := rpc.Respond(id, buf); err != nil {
				panic(err)
			}
			keysMut.RUnlock()
		}
	}()

	if _, err = rpc.Stream("query:"+eventKeyPrefix+retrieveKeysQuery, queryCh); err != nil {
		panic(err)
	}

	log.Printf("%s%s", eventKeyPrefix, wipeKeysEvent)
	if err = rpc.UserEvent(eventKeyPrefix+wipeKeysEvent, nil, true); err != nil {
		panic(err)
	}

	time.Sleep(30 * time.Second)

	keysMut.Lock()
	for i := 0; i < ahead+1; i++ {
		key := make([]byte, keySize)

		if _, err = rand.Read(key[:]); err != nil {
			panic(err)
		}

		log.Printf("%s%s %x", eventKeyPrefix, installKeyEvent, key[:nameLen:nameLen])
		if err = rpc.UserEvent(eventKeyPrefix+installKeyEvent, key[:], false); err != nil {
			panic(err)
		}

		keys = append([][]byte{key}, keys...)
	}
	keysMut.Unlock()

	time.Sleep(15 * time.Second)

	keysMut.Lock()
	log.Printf("%s%s %x", eventKeyPrefix, setDefaultKeyEvent, keys[ahead][:nameLen:nameLen])
	if err = rpc.UserEvent(eventKeyPrefix+setDefaultKeyEvent, keys[ahead][:nameLen:nameLen], false); err != nil {
		panic(err)
	}
	keysMut.Unlock()

	for range time.Tick(tick) {
		key := make([]byte, keySize)

		keysMut.Lock()
		if _, err = rand.Read(key[:]); err != nil {
			panic(err)
		}

		log.Printf("%s%s %x", eventKeyPrefix, installKeyEvent, key[:nameLen:nameLen])
		if err = rpc.UserEvent(eventKeyPrefix+installKeyEvent, key[:], false); err != nil {
			panic(err)
		}

		if len(keys) == total {
			log.Printf("%s%s %x", eventKeyPrefix, removeKeyEvent, keys[total-1][:nameLen:nameLen])
			if err = rpc.UserEvent(eventKeyPrefix+removeKeyEvent, keys[total-1][:nameLen:nameLen], false); err != nil {
				panic(err)
			}

			// zero old key
			k := keys[total-1]
			for i := range k {
				k[i] = 0
			}

			keys = append([][]byte{key}, keys[:total-1]...)
		} else {
			keys = append([][]byte{key}, keys...)
		}

		log.Printf("%s%s %x", eventKeyPrefix, setDefaultKeyEvent, keys[ahead][:nameLen:nameLen])
		if err = rpc.UserEvent(eventKeyPrefix+setDefaultKeyEvent, keys[ahead][:nameLen:nameLen], false); err != nil {
			panic(err)
		}
		keysMut.Unlock()
	}
}
