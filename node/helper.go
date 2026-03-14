package main

import (
	"log"
	"regexp"

	pb "github.com/doctor112-1/ddns-go/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func checkErrFatal(err error) {
	if err != nil {
		log.Fatalf("error %v", err)
	}
}

func connectAndVerify(addrNode string) (client pb.DdnsServiceClient, conn *grpc.ClientConn, err error) {
	var opts []grpc.DialOption

	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	conn, err = grpc.NewClient(addrNode, opts...)
	if err != nil {
		return nil, nil, err
	}

	client = pb.NewDdnsServiceClient(conn)

	return client, conn, nil
}

func verifyDomainList(domain *pb.Domain) bool {
	if matched, _ := regexp.MatchString("&", domain.Domain); matched {
		return false
	} else if matched, _ := regexp.MatchString("&", domain.Hash); matched {
		return false
	} else {
		return true
	}
}

func verifyBlock(block *pb.DomainBlock) bool {
	return true
}
