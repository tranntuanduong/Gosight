package server

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/gosight/gosight/ingestor/internal/enricher"
	"github.com/gosight/gosight/ingestor/internal/producer"
	"github.com/gosight/gosight/ingestor/internal/validation"
	pb "github.com/gosight/gosight/ingestor/proto/gosight"
)

type IngestServer struct {
	pb.UnimplementedIngestServiceServer
	producer  *producer.KafkaProducer
	validator *validation.Validator
	enricher  *enricher.Enricher
}

func NewIngestServer(p *producer.KafkaProducer, v *validation.Validator, e *enricher.Enricher) *IngestServer {
	return &IngestServer{
		producer:  p,
		validator: v,
		enricher:  e,
	}
}

func (s *IngestServer) SendEvents(stream pb.IngestService_SendEventsServer) error {
	for {
		batch, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Validate API key
		projectID, err := s.validator.ValidateAPIKey(stream.Context(), batch.ProjectKey)
		if err != nil {
			stream.Send(&pb.EventAck{
				Success:       false,
				Errors:        []string{"Invalid API key"},
				RejectedCount: int32(len(batch.Events)),
			})
			continue
		}

		// Rate limiting
		if !s.validator.CheckRateLimit(projectID) {
			stream.Send(&pb.EventAck{
				Success:       false,
				Errors:        []string{"Rate limit exceeded"},
				RejectedCount: int32(len(batch.Events)),
			})
			continue
		}

		// Process events
		accepted := 0
		rejected := 0
		var errors []string

		for _, event := range batch.Events {
			// Validate event
			if err := s.validator.ValidateEvent(event); err != nil {
				rejected++
				errors = append(errors, err.Error())
				continue
			}

			// Convert protobuf event to map for enrichment
			eventMap := s.protoEventToMap(event, projectID, batch.Session)

			// Enrich event (no user agent or IP in gRPC context by default)
			enrichedEvent := s.enricher.Enrich(eventMap, "", "")

			// Produce to Kafka
			err := s.producer.ProduceEvent(stream.Context(), projectID, enrichedEvent)
			if err != nil {
				rejected++
				errors = append(errors, err.Error())
				continue
			}

			accepted++
		}

		// Send acknowledgment
		stream.Send(&pb.EventAck{
			Success:       rejected == 0,
			AcceptedCount: int32(accepted),
			RejectedCount: int32(rejected),
			Errors:        errors,
		})
	}
}

func (s *IngestServer) protoEventToMap(event *pb.Event, projectID string, session *pb.SessionMeta) map[string]interface{} {
	eventMap := make(map[string]interface{})

	eventMap["event_id"] = event.EventId
	if eventMap["event_id"] == "" {
		eventMap["event_id"] = uuid.New().String()
	}
	eventMap["type"] = event.Type.String()
	eventMap["timestamp"] = float64(event.Timestamp)
	eventMap["project_id"] = projectID

	if session != nil {
		eventMap["session_id"] = session.SessionId
		eventMap["user_id"] = session.UserId
	}

	if event.Page != nil {
		eventMap["page"] = map[string]interface{}{
			"url":      event.Page.Url,
			"path":     event.Page.Path,
			"title":    event.Page.Title,
			"referrer": event.Page.Referrer,
		}
	}

	// Handle payload based on event type
	switch p := event.Payload.(type) {
	case *pb.Event_Click:
		eventMap["payload"] = map[string]interface{}{
			"x": p.Click.X,
			"y": p.Click.Y,
		}
		if p.Click.Target != nil {
			eventMap["target"] = map[string]interface{}{
				"tag":      p.Click.Target.Tag,
				"selector": p.Click.Target.Selector,
				"id":       p.Click.Target.Id,
				"text":     p.Click.Target.Text,
			}
		}
	case *pb.Event_Scroll:
		eventMap["payload"] = map[string]interface{}{
			"scroll_top":      p.Scroll.ScrollTop,
			"scroll_height":   p.Scroll.ScrollHeight,
			"viewport_height": p.Scroll.ViewportHeight,
			"depth_percent":   p.Scroll.DepthPercent,
		}
	case *pb.Event_JsError:
		eventMap["payload"] = map[string]interface{}{
			"message":    p.JsError.Message,
			"stack":      p.JsError.Stack,
			"source":     p.JsError.Source,
			"line":       p.JsError.Line,
			"column":     p.JsError.Column,
			"error_type": p.JsError.ErrorType,
		}
	case *pb.Event_WebVitals:
		payload := make(map[string]interface{})
		if p.WebVitals.Lcp != nil {
			payload["lcp"] = *p.WebVitals.Lcp
		}
		if p.WebVitals.Fid != nil {
			payload["fid"] = *p.WebVitals.Fid
		}
		if p.WebVitals.Cls != nil {
			payload["cls"] = *p.WebVitals.Cls
		}
		if p.WebVitals.Ttfb != nil {
			payload["ttfb"] = *p.WebVitals.Ttfb
		}
		if p.WebVitals.Fcp != nil {
			payload["fcp"] = *p.WebVitals.Fcp
		}
		if p.WebVitals.Inp != nil {
			payload["inp"] = *p.WebVitals.Inp
		}
		eventMap["payload"] = payload
	case *pb.Event_Custom:
		eventMap["payload"] = map[string]interface{}{
			"name":       p.Custom.Name,
			"properties": p.Custom.Properties,
		}
	}

	return eventMap
}

func (s *IngestServer) SendReplay(stream pb.IngestService_SendReplayServer) error {
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.ReplayAck{
				Success: true,
				Message: "All chunks received",
			})
		}
		if err != nil {
			return err
		}

		// Create chunk map for Kafka
		chunkMap := map[string]interface{}{
			"chunk_index":       chunk.ChunkIndex,
			"timestamp_start":   chunk.TimestampStart,
			"timestamp_end":     chunk.TimestampEnd,
			"data":              chunk.Data,
			"has_full_snapshot": chunk.HasFullSnapshot,
		}

		// Produce to Kafka replay topic
		// Note: gRPC ReplayChunk doesn't have session_id, using empty string
		// TODO: Update proto to include session_id for proper Kafka partitioning
		err = s.producer.ProduceReplayChunk(context.Background(), "", chunkMap)
		if err != nil {
			log.Error().Err(err).Msg("Failed to produce replay chunk")
		}
	}
}
