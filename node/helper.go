package main

import (
	"log"

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
