// Package grpc is the inbound gRPC adapter exposing the wallet hot-path balance
// operations to other services (trading, futures).
package grpc

import (
	"context"
	"log"

	"github.com/cryptox/shared/proto/walletpb"
	"github.com/cryptox/wallet-service/internal/usecase"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// Server adapts walletpb.WalletServiceServer onto the application use case.
type Server struct {
	walletpb.UnimplementedWalletServiceServer
	uc *usecase.WalletUseCase
}

func NewServer(uc *usecase.WalletUseCase) *Server { return &Server{uc: uc} }

func (s *Server) CheckBalance(ctx context.Context, req *walletpb.CheckBalanceRequest) (*walletpb.CheckBalanceResponse, error) {
	if err := s.uc.CheckBalance(ctx, uint(req.UserId), req.Currency, req.Needed); err != nil {
		return &walletpb.CheckBalanceResponse{Sufficient: false, Available: 0}, nil
	}
	return &walletpb.CheckBalanceResponse{Sufficient: true}, nil
}

func (s *Server) GetBalance(ctx context.Context, req *walletpb.GetBalanceRequest) (*walletpb.GetBalanceResponse, error) {
	bal, locked := s.uc.GetBalance(ctx, uint(req.UserId), req.Currency)
	return &walletpb.GetBalanceResponse{Balance: bal, Locked: locked}, nil
}

func (s *Server) Deduct(ctx context.Context, req *walletpb.DeductRequest) (*walletpb.DeductResponse, error) {
	if err := s.uc.AdjustBalance(ctx, uint(req.UserId), req.Currency, -req.Amount); err != nil {
		return nil, grpcstatus.Error(codes.FailedPrecondition, err.Error())
	}
	bal, _ := s.uc.GetBalance(ctx, uint(req.UserId), req.Currency)
	return &walletpb.DeductResponse{NewBalance: bal}, nil
}

func (s *Server) Credit(ctx context.Context, req *walletpb.CreditRequest) (*walletpb.CreditResponse, error) {
	if err := s.uc.AdjustBalance(ctx, uint(req.UserId), req.Currency, req.Amount); err != nil {
		log.Printf("[wallet-grpc] credit error user=%d: %v", req.UserId, err)
		return nil, grpcstatus.Error(codes.Internal, err.Error())
	}
	bal, _ := s.uc.GetBalance(ctx, uint(req.UserId), req.Currency)
	return &walletpb.CreditResponse{NewBalance: bal}, nil
}

func (s *Server) Lock(ctx context.Context, req *walletpb.LockRequest) (*walletpb.LockResponse, error) {
	if err := s.uc.LockBalance(ctx, uint(req.UserId), req.Currency, req.Amount); err != nil {
		return &walletpb.LockResponse{Success: false}, nil
	}
	return &walletpb.LockResponse{Success: true}, nil
}

func (s *Server) Unlock(ctx context.Context, req *walletpb.UnlockRequest) (*walletpb.UnlockResponse, error) {
	if err := s.uc.UnlockBalance(ctx, uint(req.UserId), req.Currency, req.Amount); err != nil {
		log.Printf("[wallet-grpc] unlock error user=%d: %v", req.UserId, err)
		return &walletpb.UnlockResponse{Success: false}, nil
	}
	return &walletpb.UnlockResponse{Success: true}, nil
}

func (s *Server) UpdateBalance(ctx context.Context, req *walletpb.UpdateBalanceRequest) (*walletpb.UpdateBalanceResponse, error) {
	if err := s.uc.AdjustBalance(ctx, uint(req.UserId), req.Currency, req.Delta); err != nil {
		return &walletpb.UpdateBalanceResponse{Success: false}, nil
	}
	return &walletpb.UpdateBalanceResponse{Success: true}, nil
}
