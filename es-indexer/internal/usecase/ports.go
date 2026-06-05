// Package usecase holds the es-indexer application logic: consume domain events
// and project them into search indices via the Indexer port.
package usecase

import "context"

// Indexer writes a document into a named index (implemented by the
// Elasticsearch adapter in infrastructure).
type Indexer interface {
	Index(ctx context.Context, index, id string, doc interface{}) error
}
