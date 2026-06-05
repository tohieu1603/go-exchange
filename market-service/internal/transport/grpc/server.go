// Package grpc is the inbound gRPC adapter exposing price/ticker reads.
package grpc

import (
	"context"

	"github.com/cryptox/market-service/internal/usecase"
	"github.com/cryptox/shared/proto/marketpb"
)

// Server adapts marketpb.MarketServiceServer onto the market query use case.
type Server struct {
	marketpb.UnimplementedMarketServiceServer
	market *usecase.MarketQuery
}

func NewServer(market *usecase.MarketQuery) *Server { return &Server{market: market} }

func (s *Server) GetPrice(ctx context.Context, req *marketpb.GetPriceRequest) (*marketpb.GetPriceResponse, error) {
	return &marketpb.GetPriceResponse{Price: s.market.Price(ctx, req.Pair)}, nil
}

func (s *Server) GetAllTickers(ctx context.Context, _ *marketpb.GetAllTickersRequest) (*marketpb.GetAllTickersResponse, error) {
	tickers := s.market.AllTickers(ctx)
	result := make([]*marketpb.Ticker, 0, len(tickers))
	for _, t := range tickers {
		result = append(result, &marketpb.Ticker{
			Pair: t.Pair, Symbol: t.Symbol, Name: t.Name,
			Price: t.Price, Change_24H: t.Change24h, Volume_24H: t.Volume24h, AssetType: t.AssetType,
		})
	}
	return &marketpb.GetAllTickersResponse{Tickers: result}, nil
}
