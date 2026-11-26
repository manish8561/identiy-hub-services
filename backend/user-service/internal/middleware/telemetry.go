package middleware

import (
	"net/http"

	"user-service/internal/telemetry"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func otelAttributeHTTPMethod(value string) attribute.KeyValue {
	return attribute.String("http.request.method", value)
}

func otelAttributeHTTPRoute(value string) attribute.KeyValue {
	return attribute.String("http.route", value)
}

func otelAttributeHTTPStatus(value int) attribute.KeyValue {
	return attribute.Int("http.response.status_code", value)
}

func OpenTelemetry() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := telemetry.Tracer("user-service/http").Start(ctx, c.Request.Method+" "+c.FullPath())
		c.Request = c.Request.WithContext(ctx)

		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			c.Request.Header.Set(TraceIDHeader, sc.TraceID().String())
			c.Writer.Header().Set(TraceIDHeader, sc.TraceID().String())
		}

		c.Next()
		span.SetAttributes(
			otelAttributeHTTPMethod(c.Request.Method),
			otelAttributeHTTPRoute(c.FullPath()),
			otelAttributeHTTPStatus(c.Writer.Status()),
		)
		if c.Writer.Status() >= http.StatusBadRequest {
			span.SetStatus(codes.Error, http.StatusText(c.Writer.Status()))
		}
		span.End()
	}
}
