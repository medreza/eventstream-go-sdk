/*
 * Copyright 2019 AccelByte Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package eventstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AccelByte/eventstream-go-sdk/v4/kafkaprometheus"
	validator "github.com/AccelByte/justice-input-validation-go"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	eventStreamNull   = "none"
	eventStreamStdout = "stdout"
	eventStreamKafka  = "kafka"

	actorTypeUser   = "USER"
	actorTypeClient = "CLIENT"
)

const (
	separator      = "." // topic prefix separator
	defaultVersion = 1
	dlq            = "dlq"
)

// log level
const (
	OffLevel   = "off"
	InfoLevel  = "info"
	DebugLevel = "debug"
	WarnLevel  = "warn"
	ErrorLevel = "error"
)

// Event defines the structure of event
type Event struct {
	ID               string                 `json:"id,omitempty"`
	EventName        string                 `json:"name,omitempty"`
	Namespace        string                 `json:"namespace,omitempty"`
	ParentNamespace  string                 `json:"parentNamespace,omitempty"`
	UnionNamespace   string                 `json:"unionNamespace,omitempty"`
	ClientID         string                 `json:"clientId,omitempty"`
	TraceID          string                 `json:"traceId,omitempty"`
	SpanContext      string                 `json:"spanContext,omitempty"`
	UserID           string                 `json:"userId,omitempty"`
	SessionID        string                 `json:"sessionId,omitempty"`
	Timestamp        string                 `json:"timestamp,omitempty"`
	Version          int                    `json:"version,omitempty"`
	EventID          int                    `json:"event_id,omitempty"`
	EventType        int                    `json:"event_type,omitempty"`
	EventLevel       int                    `json:"event_level,omitempty"`
	ServiceName      string                 `json:"service,omitempty"`
	ClientIDs        []string               `json:"client_ids,omitempty"`
	TargetUserIDs    []string               `json:"target_user_ids,omitempty"`
	TargetNamespace  string                 `json:"target_namespace,omitempty"`
	Privacy          bool                   `json:"privacy,omitempty"`
	Topic            string                 `json:"topic,omitempty"`
	AdditionalFields map[string]interface{} `json:"additional_fields,omitempty"`
	Payload          map[string]interface{} `json:"payload,omitempty"`

	Partition int    `json:",omitempty"`
	Offset    int64  `json:",omitempty"`
	Key       string `json:",omitempty"`
}

var (
	NotificationEventNamePath       = "name"
	FreeformNotificationUserIDsPath = []string{"payload", "userIds"}
)

// BrokerConfig is custom configuration for message broker
type BrokerConfig struct {
	// -- CONSUMER CONFIGS --

	// Disable auto commit on every consumer polls when the AutoCommitInterval has stepped in.
	// It's recommended to enable auto commit as manual commit per message is much slower.
	// Default: false (auto commit is enabled)
	DisableAutoCommit bool

	// Interval between auto commits. This will only take effect when auto commit is enabled.
	// Assigning zero value will be overridden by the default value.
	// Default: 1 second
	AutoCommitInterval time.Duration

	// Enable committing the message offset right after consumer polls and before the message is processed.
	// Otherwise, the message offset will be committed after it is processed. When auto commit is enabled,
	// it will store the offset to be committed by auto-committer later.
	// Default: false
	CommitBeforeProcessing bool

	// -- PUBLISHER CONFIGS --

	// The maximum time duration the client may use to deliver a message, including retries
	// Assigning zero value will be overridden by the default value.
	// Default: 60 seconds
	PublishTimeout time.Duration

	// -- GENERAL CONFIGS --

	// BaseConfig is a map to store key-value configuration of a broker.
	// It will override other configs that have been set using other BrokerConfig options.
	// Only Kafka broker is supported.
	// 		List of supported Kafka configuration: https://github.com/confluentinc/librdkafka/blob/master/CONFIGURATION.md
	BaseConfig map[string]interface{}

	StrictValidation bool
	CACertFile       string
	DialTimeout      time.Duration
	SecurityConfig   *SecurityConfig
	MetricsRegistry  prometheus.Registerer // optional registry to report metrics to prometheus (used for kafka stats)
}

// SecurityConfig contains security configuration for message broker
type SecurityConfig struct {
	AuthenticationType string
	SASLUsername       string
	SASLPassword       string
}

// PublishBuilder defines the structure of message which is sent through message broker
type PublishBuilder struct {
	id               string
	topic            string
	eventName        string
	namespace        string
	parentNamespace  string
	unionNamespace   string
	clientID         string
	traceID          string
	spanContext      string
	userID           string
	sessionID        string
	version          int
	eventID          int
	eventType        int
	eventLevel       int
	serviceName      string
	clientIDs        []string
	targetUserIDs    []string
	targetNamespace  string
	privacy          bool
	additionalFields map[string]interface{}
	key              string
	payload          map[string]interface{}
	ctx              context.Context
	timeout          time.Duration
}

// NewPublish create new PublishBuilder instance
func NewPublish() *PublishBuilder {
	return &PublishBuilder{
		version: defaultVersion,
		ctx:     context.Background(),
	}
}

// Topic set channel / topic name
func (p *PublishBuilder) Topic(topic string) *PublishBuilder {
	p.topic = topic
	return p
}

// EventName set name of published event
func (p *PublishBuilder) EventName(eventName string) *PublishBuilder {
	p.eventName = eventName
	return p
}

// Namespace set namespace of published event
func (p *PublishBuilder) Namespace(namespace string) *PublishBuilder {
	p.namespace = namespace
	return p
}

func (p *PublishBuilder) ParentNamespace(parentNamespace string) *PublishBuilder {
	p.parentNamespace = parentNamespace
	return p
}

// Parent namespace for AGS Starter, leave it empty for AGS Premium
func (p *PublishBuilder) UnionNamespace(unionNamespace string) *PublishBuilder {
	p.unionNamespace = unionNamespace
	return p
}

// ClientID set clientID of publisher event
func (p *PublishBuilder) ClientID(clientID string) *PublishBuilder {
	p.clientID = clientID
	return p
}

// TraceID set traceID of publisher event
func (p *PublishBuilder) TraceID(traceID string) *PublishBuilder {
	p.traceID = traceID
	return p
}

// SpanContext set jaeger spanContext of publisher event
func (p *PublishBuilder) SpanContext(spanID string) *PublishBuilder {
	p.spanContext = spanID
	return p
}

// SessionID set sessionID of publisher event
func (p *PublishBuilder) SessionID(sessionID string) *PublishBuilder {
	p.sessionID = sessionID
	return p
}

// UserID set userID of publisher event
func (p *PublishBuilder) UserID(userID string) *PublishBuilder {
	p.userID = userID
	return p
}

// Version set event schema version
func (p *PublishBuilder) Version(version int) *PublishBuilder {
	p.version = version
	return p
}

// EventID set eventID of publisher event
func (p *PublishBuilder) EventID(eventID int) *PublishBuilder {
	p.eventID = eventID
	return p
}

// EventType set eventType of publisher event
func (p *PublishBuilder) EventType(eventType int) *PublishBuilder {
	p.eventType = eventType
	return p
}

// EventLevel set eventLevel of publisher event
func (p *PublishBuilder) EventLevel(eventLevel int) *PublishBuilder {
	p.eventLevel = eventLevel
	return p
}

// ServiceName set serviceName of publisher event
func (p *PublishBuilder) ServiceName(serviceName string) *PublishBuilder {
	p.serviceName = serviceName
	return p
}

// ClientIDs set clientIDs of publisher event
func (p *PublishBuilder) ClientIDs(clientIDs []string) *PublishBuilder {
	p.clientIDs = clientIDs
	return p
}

// TargetUserIDs set targetUserIDs of publisher event
func (p *PublishBuilder) TargetUserIDs(targetUserIDs []string) *PublishBuilder {
	p.targetUserIDs = targetUserIDs
	return p
}

// TargetNamespace set targetNamespace of publisher event
func (p *PublishBuilder) TargetNamespace(targetNamespace string) *PublishBuilder {
	p.targetNamespace = targetNamespace
	return p
}

// Privacy set privacy of publisher event
func (p *PublishBuilder) Privacy(privacy bool) *PublishBuilder {
	p.privacy = privacy
	return p
}

// AdditionalFields set AdditionalFields of publisher event
func (p *PublishBuilder) AdditionalFields(additionalFields map[string]interface{}) *PublishBuilder {
	p.additionalFields = additionalFields
	return p
}

// ID set ID of publiser event
func (p *PublishBuilder) ID(id string) *PublishBuilder {
	p.id = id
	return p
}

// Key is a message key that used to determine the partition of the topic
// if client require strong order for the events
func (p *PublishBuilder) Key(key string) *PublishBuilder {
	p.key = key
	return p
}

// Payload is a event payload that will be published
func (p *PublishBuilder) Payload(payload map[string]interface{}) *PublishBuilder {
	p.payload = payload
	return p
}

// Context define client context when publish event.
// default: context.Background()
func (p *PublishBuilder) Context(ctx context.Context) *PublishBuilder {
	p.ctx = ctx
	return p
}

// Timeout is an upper bound on the time to report success or failure after a call to send() returns.
// The value of this config should be greater than or equal to the sum of request.timeout.ms and linger.ms.
//
// Deprecated: This config is deprecated. It will only take effect for the first publisher of the client.
// Configure PublishTimeout from the BrokerConfig instead.
//
// Default value: 60000 ms
func (p *PublishBuilder) Timeout(timeout time.Duration) *PublishBuilder {
	p.timeout = timeout
	return p
}

// SubscribeBuilder defines the structure of message which is sent through message broker
type SubscribeBuilder struct {
	topic           string
	groupID         string
	groupInstanceID string
	offset          int64
	callback        func(ctx context.Context, event *Event, err error) error
	eventName       string
	ctx             context.Context
	callbackRaw     func(ctx context.Context, msgValue []byte, err error) error
	// flag to send error message to DLQ
	sendErrorDLQ bool
	// flag to use async commit consumer
	asyncCommitMessage bool
}

// NewSubscribe create new SubscribeBuilder instance
func NewSubscribe() *SubscribeBuilder {
	return &SubscribeBuilder{
		ctx:    context.Background(),
		offset: int64(kafka.OffsetEnd),
	}
}

// Topic set topic that will be subscribe
func (s *SubscribeBuilder) Topic(topic string) *SubscribeBuilder {
	s.topic = topic
	return s
}

// Offset set Offset of the event to start
func (s *SubscribeBuilder) Offset(offset int64) *SubscribeBuilder {
	s.offset = offset
	return s
}

// GroupID set subscriber groupID. A random groupID will be generated by default.
func (s *SubscribeBuilder) GroupID(groupID string) *SubscribeBuilder {
	s.groupID = groupID
	return s
}

// GroupInstanceID set subscriber group instance ID
func (s *SubscribeBuilder) GroupInstanceID(groupInstanceID string) *SubscribeBuilder {
	s.groupInstanceID = groupInstanceID
	return s
}

// EventName set event name that will be subscribed
func (s *SubscribeBuilder) EventName(eventName string) *SubscribeBuilder {
	s.eventName = eventName
	return s
}

// Callback to do when the event received
func (s *SubscribeBuilder) Callback(
	callback func(ctx context.Context, event *Event, err error) error,
) *SubscribeBuilder {
	s.callback = callback
	return s
}

// CallbackRaw callback that receives the undecoded payload
func (s *SubscribeBuilder) CallbackRaw(
	f func(ctx context.Context, msgValue []byte, err error) error,
) *SubscribeBuilder {
	s.callbackRaw = f
	return s
}

// Context define client context when subscribe event.
// default: context.Background()
func (s *SubscribeBuilder) Context(ctx context.Context) *SubscribeBuilder {
	s.ctx = ctx
	return s
}

// SendErrorDLQ to send error message to DLQ topic.
// DLQ topic: 'topic' + -dlq
func (s *SubscribeBuilder) SendErrorDLQ(dlq bool) *SubscribeBuilder {
	s.sendErrorDLQ = dlq
	return s
}

// AsyncCommitMessage to asynchronously commit message offset.
// This setting will be overridden by AutoCommitInterval on BrokerConfig
func (s *SubscribeBuilder) AsyncCommitMessage(async bool) *SubscribeBuilder {
	s.asyncCommitMessage = async
	return s
}

// Slug is a string describing a unique subscriber (topic, eventName, groupID)
func (s *SubscribeBuilder) Slug() string {
	return fmt.Sprintf("%s%s%s%s%s", s.topic, kafkaprometheus.SlugSeparator, s.eventName, kafkaprometheus.SlugSeparator, s.groupID)
}

func NewClient(prefix, stream string, brokers []string, config ...*BrokerConfig) (Client, error) {
	switch stream {
	case eventStreamNull:
		return newBlackholeClient(), nil
	case eventStreamStdout:
		return newStdoutClient(prefix), nil
	case eventStreamKafka:
		return newKafkaClient(brokers, prefix, config...)
	default:
		return nil, errors.New("unsupported stream")
	}
}

// Client is an interface for event stream functionality
type Client interface {
	Publish(publishBuilder *PublishBuilder) error
	PublishSync(publishBuilder *PublishBuilder) error
	Register(subscribeBuilder *SubscribeBuilder) error
	PublishAuditLog(auditLogBuilder *AuditLogBuilder) error
	GetMetadata(topic string, timeout time.Duration) (*Metadata, error)
}

type AuditLog struct {
	ID              string          `json:"_id" valid:"required"`
	Category        string          `json:"category" valid:"required"`
	ActionName      string          `json:"actionName" valid:"required"`
	Timestamp       int64           `json:"timestamp" valid:"required"`
	IP              string          `json:"ip,omitempty" valid:"optional"`
	Actor           string          `json:"actor" valid:"uuid4WithoutHyphens,required"`
	ActorType       string          `json:"actorType" valid:"required~actorType values: USER CLIENT"`
	ClientID        string          `json:"clientId" valid:"uuid4WithoutHyphens,required"`
	ActorNamespace  string          `json:"actorNamespace" valid:"required"`
	ObjectID        string          `json:"objectId,omitempty" valid:"optional"`
	ObjectType      string          `json:"objectType,omitempty" valid:"optional"`
	ObjectNamespace string          `json:"objectNamespace" valid:"required~use publisher namespace if resource has no namespace"`
	TargetUserID    string          `json:"targetUserId,omitempty" valid:"uuid4WithoutHyphens,optional"`
	DeviceID        string          `json:"deviceId,omitempty" valid:"optional"`
	Payload         AuditLogPayload `json:"payload" valid:"required"`
}

type PublishErrorCallbackFunc func(message []byte, err error)

type AuditLogBuilder struct {
	category        string                 `description:"required"`
	actionName      string                 `description:"required"`
	ip              string                 `description:"optional"`
	actor           string                 `description:"uuid4WithoutHyphens,required"`
	actorType       string                 `description:"required~actorType values: USER CLIENT"`
	clientID        string                 `description:"uuid4WithoutHyphens,required"`
	actorNamespace  string                 `description:"required"`
	objectID        string                 `description:"optional"`
	objectType      string                 `description:"optional"`
	objectNamespace string                 `description:"required~use publisher namespace if resource has no namespace"`
	targetUserID    string                 `description:"uuid4WithoutHyphens,optional"`
	deviceID        string                 `description:"optional"`
	content         map[string]interface{} `description:"optional"`
	diff            *AuditLogDiff          `description:"optional, if diff is not nil, please make sure diff.Before and diff.Before are both not nil"`

	key     string
	ctx     context.Context
	version int
}

// NewAuditLogBuilder create new AuditLogBuilder instance
func NewAuditLogBuilder() *AuditLogBuilder {
	return &AuditLogBuilder{
		version: defaultVersion,
		ctx:     context.Background(),
	}
}

type AuditLogPayload struct {
	Content map[string]interface{} `json:"content"`
	Diff    AuditLogDiff           `json:"diff"`
}

type AuditLogDiff struct {
	Before map[string]interface{} `json:"before,omitempty"`
	After  map[string]interface{} `json:"after,omitempty"`
}

func (auditLogBuilder *AuditLogBuilder) Category(category string) *AuditLogBuilder {
	auditLogBuilder.category = category
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) ActionName(actionName string) *AuditLogBuilder {
	auditLogBuilder.actionName = actionName
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) IP(ip string) *AuditLogBuilder {
	auditLogBuilder.ip = ip
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) Actor(actor string) *AuditLogBuilder {
	auditLogBuilder.actor = actor
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) IsActorTypeUser(isActorTypeUser bool) *AuditLogBuilder {
	if isActorTypeUser {
		auditLogBuilder.actorType = actorTypeUser
	} else {
		auditLogBuilder.actorType = actorTypeClient
	}

	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) ClientID(clientID string) *AuditLogBuilder {
	auditLogBuilder.clientID = clientID
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) ActorNamespace(actorNamespace string) *AuditLogBuilder {
	auditLogBuilder.actorNamespace = actorNamespace
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) ObjectID(objectID string) *AuditLogBuilder {
	auditLogBuilder.objectID = objectID
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) ObjectType(objectType string) *AuditLogBuilder {
	auditLogBuilder.objectType = objectType
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) ObjectNamespace(objectNamespace string) *AuditLogBuilder {
	auditLogBuilder.objectNamespace = objectNamespace
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) TargetUserID(targetUserID string) *AuditLogBuilder {
	auditLogBuilder.targetUserID = targetUserID
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) DeviceID(deviceID string) *AuditLogBuilder {
	auditLogBuilder.deviceID = deviceID
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) Content(content map[string]interface{}) *AuditLogBuilder {
	auditLogBuilder.content = content
	return auditLogBuilder
}

// Diff If diff is not nil, please make sure diff.Before and diff.Before are both not nil
func (auditLogBuilder *AuditLogBuilder) Diff(diff *AuditLogDiff) *AuditLogBuilder {
	auditLogBuilder.diff = diff
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) Key(key string) *AuditLogBuilder {
	auditLogBuilder.key = key
	return auditLogBuilder
}

func (auditLogBuilder *AuditLogBuilder) Build() (*kafka.Message, error) {

	id := generateID()
	auditLog := &AuditLog{
		ID:              id,
		Category:        auditLogBuilder.category,
		ActionName:      auditLogBuilder.actionName,
		Timestamp:       time.Now().UnixMilli(),
		IP:              auditLogBuilder.ip,
		Actor:           auditLogBuilder.actor,
		ActorType:       auditLogBuilder.actorType,
		ClientID:        auditLogBuilder.clientID,
		ActorNamespace:  auditLogBuilder.actorNamespace,
		ObjectID:        auditLogBuilder.objectID,
		ObjectType:      auditLogBuilder.objectType,
		ObjectNamespace: auditLogBuilder.objectNamespace,
		TargetUserID:    auditLogBuilder.targetUserID,
		DeviceID:        auditLogBuilder.deviceID,
	}
	var content map[string]interface{}
	if auditLogBuilder.content == nil {
		content = make(map[string]interface{})
	} else {
		content = auditLogBuilder.content
	}
	diff := AuditLogDiff{}
	if auditLogBuilder.diff != nil {
		diff = *auditLogBuilder.diff
	}
	payload := AuditLogPayload{
		Content: content,
		Diff:    diff,
	}
	auditLog.Payload = payload

	valid, err := validator.ValidateStruct(auditLog)
	if err != nil {
		logrus.WithField("action", auditLog.ActionName).
			Errorf("unable to validate audit log. error : %v", err)
		return &kafka.Message{}, err
	}
	if !valid {
		return &kafka.Message{}, errInvalidPubStruct
	}

	auditLogBytes, marshalErr := json.Marshal(auditLog)
	if marshalErr != nil {
		logrus.WithField("action", auditLog.ActionName).
			Errorf("unable to marshal audit log : %v, error: %v", auditLog, marshalErr)
		return &kafka.Message{}, marshalErr
	}

	return &kafka.Message{
		Key:   []byte(auditLogBuilder.key),
		Value: auditLogBytes,
	}, nil
}
