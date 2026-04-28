package grpc

import (
	"context"

	"github.com/cryptox/market-service/internal/service"
	"github.com/cryptox/shared/proto/marketpb"
)

type MarketGRPCServer struct {
	marketpb.UnimplementedMarketServiceServer
	priceFeed *service.PriceFeed
}

func NewMarketGRPCServer(pf *service.PriceFeed) *MarketGRPCServer {
	return &MarketGRPCServer{priceFeed: pf}
}

func (s *MarketGRPCServer) GetPrice(ctx context.Context, req *marketpb.GetPriceRequest) (*marketpb.GetPriceResponse, error) {
	price := s.priceFeed.GetPrice(req.Pair)
	return &marketpb.GetPriceResponse{Price: price}, nil
}

func (s *MarketGRPCServer) GetAllTickers(ctx context.Context, req *marketpb.GetAllTickersRequest) (*marketpb.GetAllTickersResponse, error) {
	tickers := s.priceFeed.GetAllTickers()
	result := make([]*marketpb.Ticker, 0, len(tickers))
	for _, t := range tickers {
		ticker := &marketpb.Ticker{}
		if v, ok := t["pair"].(string); ok {
			ticker.Pair = v
		}
		if v, ok := t["symbol"].(string); ok {
			ticker.Symbol = v
		}
		if v, ok := t["name"].(string); ok {
			ticker.Name = v
		}
		if v, ok := t["price"].(float64); ok {
			ticker.Price = v
		}
		if v, ok := t["change24h"].(float64); ok {
			ticker.Change_24H = v
		}
		if v, ok := t["volume24h"].(float64); ok {
			ticker.Volume_24H = v
		}
		if v, ok := t["assetType"].(string); ok {
			ticker.AssetType = v
		}
		result = append(result, ticker)
	}
	return &marketpb.GetAllTickersResponse{Tickers: result}, nil
}
