package grpc

import (
	"context"
	"log"

	"github.com/cryptox/shared/proto/walletpb"
	"github.com/cryptox/wallet-service/internal/service"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type WalletGRPCServer struct {
	walletpb.UnimplementedWalletServiceServer
	walletSvc *service.WalletService
}

func NewWalletGRPCServer(svc *service.WalletService) *WalletGRPCServer {
	return &WalletGRPCServer{walletSvc: svc}
}

func (s *WalletGRPCServer) CheckBalance(ctx context.Context, req *walletpb.CheckBalanceRequest) (*walletpb.CheckBalanceResponse, error) {
	err := s.walletSvc.CheckBalanceRedis(ctx, uint(req.UserId), req.Currency, req.Needed)
	if err != nil {
		return &walletpb.CheckBalanceResponse{Sufficient: false, Available: 0}, nil
	}
	return &walletpb.CheckBalanceResponse{Sufficient: true}, nil
}

func (s *WalletGRPCServer) GetBalance(ctx context.Context, req *walletpb.GetBalanceRequest) (*walletpb.GetBalanceResponse, error) {
	bal, locked := s.walletSvc.GetBalanceRedis(ctx, uint(req.UserId), req.Currency)
	return &walletpb.GetBalanceResponse{Balance: bal, Locked: locked}, nil
}

func (s *WalletGRPCServer) Deduct(ctx context.Context, req *walletpb.DeductRequest) (*walletpb.DeductResponse, error) {
	err := s.walletSvc.UpdateBalanceRedis(ctx, uint(req.UserId), req.Currency, -req.Amount)
	if err != nil {
		return nil, grpcstatus.Error(codes.FailedPrecondition, err.Error())
	}
	bal, _ := s.walletSvc.GetBalanceRedis(ctx, uint(req.UserId), req.Currency)
	return &walletpb.DeductResponse{NewBalance: bal}, nil
}

func (s *WalletGRPCServer) Credit(ctx context.Context, req *walletpb.CreditRequest) (*walletpb.CreditResponse, error) {
	err := s.walletSvc.UpdateBalanceRedis(ctx, uint(req.UserId), req.Currency, req.Amount)
	if err != nil {
		log.Printf("[wallet-grpc] credit error user=%d: %v", req.UserId, err)
		return nil, grpcstatus.Error(codes.Internal, err.Error())
	}
	bal, _ := s.walletSvc.GetBalanceRedis(ctx, uint(req.UserId), req.Currency)
	return &walletpb.CreditResponse{NewBalance: bal}, nil
}

func (s *WalletGRPCServer) Lock(ctx context.Context, req *walletpb.LockRequest) (*walletpb.LockResponse, error) {
	err := s.walletSvc.LockBalanceRedis(ctx, uint(req.UserId), req.Currency, req.Amount)
	if err != nil {
		return &walletpb.LockResponse{Success: false}, nil
	}
	return &walletpb.LockResponse{Success: true}, nil
}

func (s *WalletGRPCServer) Unlock(ctx context.Context, req *walletpb.UnlockRequest) (*walletpb.UnlockResponse, error) {
	err := s.walletSvc.UnlockBalanceRedis(ctx, uint(req.UserId), req.Currency, req.Amount)
	if err != nil {
		log.Printf("[wallet-grpc] unlock error user=%d: %v", req.UserId, err)
		return &walletpb.UnlockResponse{Success: false}, nil
	}
	return &walletpb.UnlockResponse{Success: true}, nil
}

func (s *WalletGRPCServer) UpdateBalance(ctx context.Context, req *walletpb.UpdateBalanceRequest) (*walletpb.UpdateBalanceResponse, error) {
	err := s.walletSvc.UpdateBalanceRedis(ctx, uint(req.UserId), req.Currency, req.Delta)
	if err != nil {
		return &walletpb.UpdateBalanceResponse{Success: false}, nil
	}
	return &walletpb.UpdateBalanceResponse{Success: true}, nil
}
