package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	pb "github.com/doctor112-1/ddns-go/proto"
	"google.golang.org/grpc"
)

var port = flag.Int("port", 3001, "server port")

type server struct {
	pb.UnimplementedDdnsServiceServer
}

func (s *server) GetDomainList(in *pb.Ask, stream pb.DdnsService_GetDomainListServer) error {
	if err := stream.Send(&pb.Domain{Domain: "example.com", Hash: "a379a6f6eeafb9a55e378c118034e2751e682fab9f2d30ab13d2125586ce1947"}); err != nil {
		return err
	}

	if err := stream.Send(&pb.Domain{Domain: "example.com", Hash: "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}); err != nil {
		return err
	}

	if err := stream.Send(&pb.Domain{Domain: "hamrat.com", Hash: "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"}); err != nil {
		return err
	}

	return nil
}

func (s *server) GetID(ctx context.Context, ask *pb.Ask) (*pb.PeerID, error) {
	return &pb.PeerID{Id: "superduperfunid"}, nil
}

func (s *server) PeersKnown(ctx context.Context, ask *pb.Ask) (*pb.NumOfPeersKnown, error) {
	return &pb.NumOfPeersKnown{Num: 2}, nil
}

func (s *server) FetchDomain(ctx context.Context, domain *pb.Domain) (*pb.DomainBlock, error) {
	return &pb.DomainBlock{Blockheaders: &pb.Blockheaders{Timestamp: 1, Nonce: 1, Target: 1, DomainName: "example.com"}, PublicKey: "awesome"}, nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterDdnsServiceServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
