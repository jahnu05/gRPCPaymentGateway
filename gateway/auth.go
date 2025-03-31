package gateway

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)


// authInterceptor verifies metadata credentials by looking up the registered user's password.
func authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if info.FullMethod == "/payment.PaymentGateway/Register" {
		return handler(ctx, req)
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(16, "missing metadata")
	}
	usernames := md["username"]
	passwords := md["password"]
	if len(usernames) == 0 || len(passwords) == 0 {
		return nil, status.Errorf(16, "missing credentials")
	}
	username := usernames[0]
	providedPwd := passwords[0]
	val, exists := gatewayInstance.users.Load(username)
	if !exists {
		return nil, status.Errorf(16, "user not registered")
	}
	regUser := val.(registeredUser)
	if providedPwd != regUser.password {
		return nil, status.Errorf(7, "invalid credentials")
	}
	return handler(ctx, req)
}

// authorizationInterceptor ensures that for GetBalance and GetTransactionHistory requests,
// the user can only view their own information.
func authorizationInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if info.FullMethod == "/payment.PaymentGateway/GetBalance" || info.FullMethod == "/payment.PaymentGateway/GetTransactionHistory" {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(16, "missing metadata")
		}
		usernames := md["username"]
		if len(usernames) == 0 {
			return nil, status.Errorf(16, "missing username")
		}
		authenticatedUser := usernames[0]
		switch r := req.(type) {
		case *paymentpb.BalanceRequest:
			if r.Username != authenticatedUser {
				return nil, status.Errorf(7, "unauthorized: %s cannot view balance for %s", authenticatedUser, r.Username)
			}
		case *paymentpb.HistoryRequest:
			if r.Username != authenticatedUser {
				return nil, status.Errorf(7, "unauthorized: %s cannot view transaction history for %s", authenticatedUser, r.Username)
			}
		default:
			return nil, status.Errorf(13, "invalid request type")
		}
	}
	return handler(ctx, req)
}

// loggingInterceptor logs every request, response, and client certificate details.
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if p, ok := peer.FromContext(ctx); ok {
		if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			if len(tlsInfo.State.PeerCertificates) > 0 {
				clientCert := tlsInfo.State.PeerCertificates[0]
				log.Printf("Client certificate subject: %s", clientCert.Subject)
			}
		}
	}
	log.Printf("Request: Method=%s, Payload=%v", info.FullMethod, req)
	resp, err := handler(ctx, req)
	log.Printf("Response: Method=%s, Response=%v, Error=%v", info.FullMethod, resp, err)
	return resp, err
}
