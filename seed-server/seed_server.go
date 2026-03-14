package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	pb "github.com/doctor112-1/ddns-go/proto"
	"google.golang.org/grpc"
)

var port = flag.Int("port", 3000, "server port")

type server struct {
	pb.UnimplementedDdnsServiceServer
}

// TODO: small network
func (s *server) GetNodes(in *pb.AskNodes, stream pb.DdnsService_GetNodesServer) error {
	if in.NumberOfNodes > 9 {
		if err := stream.Send(&pb.Nodes{Message: &pb.Nodes_Error{Error: "too many nodes requested"}}); err != nil {
			return err
		}
		return nil
	}

	for i := 0; i < int(in.NumberOfNodes); i++ {
		if err := stream.Send(&pb.Nodes{Message: &pb.Nodes_Ip{Ip: "127.0.0.1:3001"}}); err != nil {
			return err
		}
	}

	return nil
}

// TODO: implement rpc to get ip

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
