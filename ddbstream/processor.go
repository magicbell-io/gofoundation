// Package ddbstream provides support for converting dynamodb stream events to core events.
package ddbstream

import (
	"context"
	"fmt"
	"strings"

	typesStream "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
	"github.com/code-inbox/mason-go/ddb"
	"github.com/google/uuid"
)

// Processor represents the ddbstream processor.
type Processor struct {
	ddb      *ddb.Store
	handlers map[string]map[string][]HandleFunc
}

var mapping = map[typesStream.OperationType]string{
	typesStream.OperationTypeInsert: "created",
	typesStream.OperationTypeModify: "updated",
	typesStream.OperationTypeRemove: "deleted",
}

func NewProcessor(ddb *ddb.Store) *Processor {
	return &Processor{
		ddb:      ddb,
		handlers: map[string]map[string][]HandleFunc{},
	}
}

func (p *Processor) Process(ctx context.Context, records []*typesStream.Record) error {
	for _, record := range records {
		keys := record.Dynamodb.Keys
		pk := keys["PK"].(*typesStream.AttributeValueMemberS)
		sk := keys["SK"].(*typesStream.AttributeValueMemberS)

		parts := strings.Split(pk.Value, "#")
		if len(parts) != 2 {
			return fmt.Errorf("invalid pk: %s", pk.Value)
		}

		src := parts[0]
		id, err := uuid.Parse(parts[1])
		if err != nil {
			return fmt.Errorf("ID is not UUID: %s", parts[1])
		}

		parts = strings.Split(sk.Value, "#")
		skSrc := parts[0]

		switch {
		case src == skSrc:
			evt := Event{
				Source: src,
				Type:   mapping[record.EventName],
				ID:     id,
				PK:     pk.Value,
				SK:     sk.Value,
			}

			handlers, exist := p.handlers[evt.Source][evt.Type]
			if !exist {
				continue
			}

			for _, handler := range handlers {
				if err := handler(ctx, evt); err != nil {
					return err
				}
			}
		default:
			// skip events on items with composite keys
			return nil
		}
	}

	return nil
}

// AddHandler adds a handler for a source and type.
func (p *Processor) RegisterHandler(source string, evtType string, fn HandleFunc) {
	handlers, exist := p.handlers[source]
	if !exist {
		handlers = map[string][]HandleFunc{}
	}
	p.handlers[source] = handlers

	handlers[evtType] = append(handlers[evtType], fn)
}
