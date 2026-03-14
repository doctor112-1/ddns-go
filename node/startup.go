package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/dgraph-io/badger/v4"
	pb "github.com/doctor112-1/ddns-go/proto"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

func exists() bool {
	dir, err := os.UserHomeDir()
	dir = filepath.Join(dir, ".ddns-go")
	checkErrFatal(err)

	_, err = os.Stat(dir)
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(dir, "peers"))
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(dir, "all-domains"))
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(dir, "domains"))
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(dir, "id"))
	if err != nil {
		return false
	}

	return true
}

var repeatedDomains []string

func newStartup() {
	dir, err := os.UserHomeDir()
	checkErrFatal(err)

	dir = filepath.Join(dir, ".ddns-go")

	os.RemoveAll(dir)

	err = os.Mkdir(dir, 0o755)
	checkErrFatal(err)

	id := generateID()

	listOfNodes := fetchNodesSeedServer()

	optsAllDomains := badger.DefaultOptions(filepath.Join(dir, "all-domains"))
	optsAllDomains.Logger = nil
	dbAllDomains, err := badger.Open(optsAllDomains)
	checkErrFatal(err)
	defer dbAllDomains.Close()

	getAllDomains(listOfNodes, dbAllDomains)
	sortAllDomains(dbAllDomains)
	// we get highestDiff and lowestDiff from cleanDomains so we don't need to run a for loop and get it again
	highestDiff, lowestDiff := cleanDomains(dbAllDomains, id)

	optsPeers := badger.DefaultOptions(filepath.Join(dir, "peers"))
	optsPeers.Logger = nil
	dbPeers, err := badger.Open(optsPeers)
	checkErrFatal(err)
	defer dbPeers.Close()

	getStartPeerIDS(listOfNodes, dbPeers)

	// get close domains
	optsDomains := badger.DefaultOptions(filepath.Join(dir, "/domains"))
	optsDomains.Logger = nil
	dbDomains, err := badger.Open(optsDomains)
	checkErrFatal(err)
	defer dbDomains.Close()

	getCloseDomains(dbDomains, dbAllDomains, listOfNodes, highestDiff, lowestDiff, id)
}

func getCloseDomains(dbDomains *badger.DB, dbAllDomains *badger.DB, listOfNodes []string, highestDiff *big.Int, lowestDiff *big.Int, peerID []byte) {
	avgNum := 0
	for _, addrNode := range listOfNodes {
		client, conn, err := connectAndVerify(addrNode)

		num, err := client.PeersKnown(context.Background(), &pb.Ask{Ask: true})
		if err != nil {
			log.Printf("\033[31m Error: %v \033[0m", err)
			return
		}

		avgNum += int(num.Num)
		conn.Close()
	}

	// average amount of nodes known
	avgNum = avgNum / len(listOfNodes)

	if (avgNum == 0) || (avgNum == 1) {
		// fetch all domains
		dbAllDomains.Update(func(txn *badger.Txn) error {
			dbADIter := txn.NewIterator(badger.DefaultIteratorOptions)
			defer dbADIter.Close()

			var opts []grpc.DialOption

			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

			var conn *grpc.ClientConn
			var client pb.DdnsServiceClient
			var err error

			for _, addrNode := range listOfNodes {
				client, conn, err = connectAndVerify(addrNode)
				_, err = client.GetID(context.Background(), &pb.Ask{Ask: true})
				if err == nil {
					break
				}
			}

			for dbADIter.Rewind(); dbADIter.Valid(); dbADIter.Next() {
				item := dbADIter.Item()

				key := item.Key()

				valStr := ""
				err := item.Value(func(val []byte) error {
					valStr = string(val)
					return nil
				})
				checkErrFatal(err)

				domainBlock, _ := client.FetchDomain(context.Background(), &pb.Domain{Domain: string(key), Hash: valStr})
				domainBlockByte, _ := proto.Marshal(domainBlock)
				dbDomains.Update(func(txn *badger.Txn) error {
					txn.Set([]byte(valStr), domainBlockByte)
					return nil
				})
			}
			conn.Close()
			return nil
		})
	} else {

		space := new(big.Int)
		space = space.Add(lowestDiff, highestDiff)
		space = space.Div(space, new(big.Int).SetInt64(int64(avgNum)))
		idNum := new(big.Int)
		idNum.SetBytes(peerID)

		var bounds [2]*big.Int

		if idNum.Cmp(lowestDiff) <= 0 {
			bounds[0] = lowestDiff
			bounds[1] = new(big.Int).Add(lowestDiff, space)
		}

		if idNum.Cmp(highestDiff) >= 0 {
			bounds[0] = new(big.Int).Sub(highestDiff, space)
			bounds[1] = highestDiff
		}

		for inc := new(big.Int).Set(lowestDiff); inc.Cmp(highestDiff) < 0; inc = inc.Add(inc, space) {
			ahead := new(big.Int).Add(inc, space)
			if (inc.Cmp(idNum) <= 0) && (idNum.Cmp(ahead) <= 0) {
				if ahead.Cmp(highestDiff) == 1 {
					rem := new(big.Int).Sub(ahead, highestDiff)
					bounds[0] = new(big.Int).Sub(inc, rem)
					bounds[1] = new(big.Int).Set(highestDiff)
					break
				} else {
					bounds[0] = new(big.Int).Set(inc)
					bounds[1] = ahead
					break
				}
			}
		}

		var conn *grpc.ClientConn
		var client pb.DdnsServiceClient
		var err error

		for _, addrNode := range listOfNodes {
			client, conn, err = connectAndVerify(addrNode)
			_, err = client.GetID(context.Background(), &pb.Ask{Ask: true})
			if err == nil {
				break
			}
		}

		dbAllDomains.Update(func(txn *badger.Txn) error {
			dbADIter := txn.NewIterator(badger.DefaultIteratorOptions)
			defer dbADIter.Close()

			for dbADIter.Rewind(); dbADIter.Valid(); dbADIter.Next() {
				item := dbADIter.Item()

				key := item.Key()

				valStr := ""
				err := item.Value(func(val []byte) error {
					valStr = string(val)
					return nil
				})
				if err != nil {
					log.Fatalf("error: %v", err)
				}

				decodedHash, _ := hex.DecodeString(valStr)
				hashNum := new(big.Int)
				hashNum.SetBytes(decodedHash)

				if (bounds[0].Cmp(hashNum) == -1) && (bounds[1].Cmp(hashNum) == 1) {
					domainBlock, _ := client.FetchDomain(context.Background(), &pb.Domain{Domain: string(key), Hash: valStr})
					domainBlockByte, _ := proto.Marshal(domainBlock)
					dbDomains.Update(func(txn *badger.Txn) error {
						txn.Set([]byte(valStr), domainBlockByte)
						return nil
					})
				}

			}
			return nil
		})
		conn.Close()
	}
}

func getStartPeerIDS(listOfNodes []string, db *badger.DB) {
	for _, node := range listOfNodes {
		peerID := getPeerID(node)
		db.Update(func(txn *badger.Txn) error {
			txn.Set([]byte(peerID), []byte(node))
			return nil
		})
	}
}

func getPeerID(addrNode string) (peerID string) {
	client, conn, err := connectAndVerify(addrNode)
	if err != nil {
		log.Printf("\033[31m Error: %v \033[0m", err)
		return
	}

	id, err := client.GetID(context.Background(), &pb.Ask{Ask: true})
	if err != nil {
		log.Printf("\033[31m Error: %v \033[0m", err)
		return
	}

	conn.Close()

	return id.Id
}

func generateID() []byte {
	dir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("err: %v", err)
	}
	dir = dir + "/.ddns-go"

	uuid := uuid.New()

	h := sha256.New()

	h.Write([]byte(uuid.String()))

	id := h.Sum(nil)

	os.WriteFile(dir+"/id", id, 0o755)

	return id
}

func cleanDomains(db *badger.DB, peerID []byte) (*big.Int, *big.Int) {
	idNum := new(big.Int)
	idNum.SetBytes(peerID)
	highest := new(big.Int)
	lowest := new(big.Int)
	lowest.SetString("10000000000000000000000000000000000000000000000000000000000000000000000000000000", 10)
	db.Update(func(txn *badger.Txn) error {
		dbIter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer dbIter.Close()

		for dbIter.Rewind(); dbIter.Valid(); dbIter.Next() {
			item := dbIter.Item()
			key := item.Key()

			domain, hash, _ := strings.Cut(string(key), "&&&")

			txn.Delete(key)
			txn.Set([]byte(domain), []byte(hash))

			// bundle finding the highest difference hash and lowest difference hash so no need for for loop later
			decodedHash, _ := hex.DecodeString(hash)
			hashNum := new(big.Int)
			hashNum.SetBytes(decodedHash)
			diff := new(big.Int)
			diff.Sub(hashNum, idNum)
			diff = diff.Abs(diff)

			if diff.Cmp(highest) == 1 {
				highest = diff
			}

			if diff.Cmp(lowest) == -1 {
				lowest = diff
			}
		}
		return nil
	})

	return highest, lowest
}

// TODO: make something to handle when they have equal reputation
func sortAllDomains(db *badger.DB) {
	for _, domain := range repeatedDomains {
		db.Update(func(txn *badger.Txn) error {
			dbIter := txn.NewIterator(badger.DefaultIteratorOptions)
			defer dbIter.Close()

			prefix := []byte(domain)

			winKey := []byte("a")
			winRep := 0
			winVal := []byte("a")

			for dbIter.Seek(prefix); dbIter.ValidForPrefix(prefix); dbIter.Next() {
				item := dbIter.Item()

				key := item.Key()

				valStr := "a"
				err := item.Value(func(val []byte) error {
					valStr = string(val)
					return nil
				})
				if err != nil {
					log.Fatalf("error: %v", err)
				}
				currRep, err := strconv.Atoi(valStr[0:1])
				if err != nil {
					log.Fatalf("error: %v", err)
				}

				if currRep > winRep {
					winRep = currRep
					winKey = key
					winVal = []byte(valStr)
				}

				txn.Delete(key)
			}

			txn.Set(winKey, winVal)
			return nil
		})
	}
}

// gets all domains
func getAllDomains(listOfNodes []string, db *badger.DB) {
	for _, node := range listOfNodes {
		getDomainList(node, db)
	}
}

// gets the domain list from an individual node
func getDomainList(addrNode string, db *badger.DB) {
	// TODO: validate blocks and make sure they don't contain things like &
	client, conn, err := connectAndVerify(addrNode)
	if err != nil {
		log.Printf("\033[31m Error: %v \033[0m", err)
		return
	}

	stream, err := client.GetDomainList(context.Background(), &pb.Ask{Ask: true})
	if err != nil {
		log.Printf("\033[31m Error: %v \033[0m", err)
		return
	}

	for {
		domain, err := stream.Recv()

		if err == io.EOF {
			break
		}

		if err != nil {
			log.Printf("\033[31m Error: %v \033[0m", err)
			return
		}

		if verifyDomainList(domain) {
			// one db - key is domain name&&&hash, value is reputation&&&ipaddr

			err = db.Update(func(txn *badger.Txn) error {
				item, _ := txn.Get([]byte(domain.Domain + "&&&" + domain.Hash))
				if item == nil {
					opts := badger.DefaultIteratorOptions
					opts.PrefetchValues = false
					dbIter := txn.NewIterator(opts)
					defer dbIter.Close()
					prefix := []byte(domain.Domain)
					for dbIter.Seek(prefix); dbIter.ValidForPrefix(prefix); dbIter.Next() {
						item := dbIter.Item()

						if item != nil {
							repeatedDomains = append(repeatedDomains, domain.Domain)
						}
					}
					txn.Set([]byte(domain.Domain+"&&&"+domain.Hash), []byte("1"+"&&&"+addrNode))
				} else {
					// increment reputation and add ip address at the end
					itemVal := "a"
					item.Value(func(val []byte) error {
						itemVal = string(val)
						return nil
					})

					if matched, _ := regexp.MatchString(addrNode, itemVal); !matched {
						rep, err := strconv.Atoi(itemVal[0:1])
						if err != nil {
							return err
						}
						rep++
						itemVal = strconv.Itoa(rep) + itemVal[1:] + "&&&" + addrNode

						txn.Set([]byte(domain.Domain+"&&&"+domain.Hash), []byte(itemVal))
					}

				}
				return nil
			})
			checkErrFatal(err)
		}

		conn.Close()
	}
}

// TODO: small network
func fetchNodesSeedServer() (listOfNodes []string) {
	if *nodesToFetch > 9 {
		log.Fatalln("error: too many nodes")
	}

	client, conn, err := connectAndVerify("localhost:3000")
	checkErrFatal(err)

	stream, err := client.GetNodes(context.Background(), &pb.AskNodes{NumberOfNodes: uint32(*nodesToFetch)})
	checkErrFatal(err)

	for {
		ip, err := stream.Recv()
		if err == io.EOF {
			break
		}

		checkErrFatal(err)

		listOfNodes = append(listOfNodes, ip.GetIp())
	}

	conn.Close()

	return listOfNodes
}
