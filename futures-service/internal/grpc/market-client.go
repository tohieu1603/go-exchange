package grpc

import (
	"context"

	"github.com/cryptox/shared/grpcutil"
	"github.com/cryptox/shared/proto/marketpb"
)

type MarketClient struct {
	client marketpb.MarketServiceClient
}

func NewMarketClient(addr string) *MarketClient {
	conn := grpcutil.Dial(addr)
	return &MarketClient{client: marketpb.NewMarketServiceClient(conn)}
}

func (c *MarketClient) GetPrice(ctx context.Context, pair string) (float64, error) {
	resp, err := c.client.GetPrice(ctx, &marketpb.GetPriceRequest{Pair: pair})
	if err != nil {
		return 0, err
	}
	return resp.Price, nil
}
