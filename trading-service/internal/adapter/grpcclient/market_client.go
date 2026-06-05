package grpcclient

import (
	"context"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/marketpb"
)

// MarketClient implements application.PriceGateway over gRPC to market-service.
type MarketClient struct {
	client marketpb.MarketServiceClient
}

func NewMarketClient(addr string) *MarketClient {
	return &MarketClient{client: marketpb.NewMarketServiceClient(grpcutil.Dial(addr))}
}

func (c *MarketClient) GetPrice(ctx context.Context, pair string) (float64, error) {
	resp, err := c.client.GetPrice(ctx, &marketpb.GetPriceRequest{Pair: pair})
	if err != nil {
		return 0, err
	}
	return resp.Price, nil
}
