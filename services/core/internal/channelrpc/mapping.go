package channelrpc

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/monkeylabx/threadline/services/core/internal/dbgen"
	channelv1 "github.com/monkeylabx/threadline/services/gen/threadline/channel/v1"
)

// toProto maps a stored Channel row to the wire message. Topic, retention and
// agent policy are not stored yet and stay unset; unknown stored codes map to
// UNSPECIFIED rather than guessing.
func toProto(channel dbgen.DomainChannel) *channelv1.Channel {
	message := &channelv1.Channel{
		ChannelId:   channel.ChannelID,
		TenantId:    channel.TenantID,
		SpaceId:     channel.SpaceID,
		Name:        channel.Name,
		Visibility:  protoVisibility(channel.Visibility),
		State:       protoState(channel.State),
		E2EeGroupId: channel.E2eeGroupID,
	}
	if channel.CreatedAt.Valid {
		message.CreatedAt = timestamppb.New(channel.CreatedAt.Time)
	}
	return message
}

func protoVisibility(stored int16) channelv1.ChannelVisibility {
	switch stored {
	case 1:
		return channelv1.ChannelVisibility_CHANNEL_VISIBILITY_PUBLIC
	case 2:
		return channelv1.ChannelVisibility_CHANNEL_VISIBILITY_PRIVATE
	default:
		return channelv1.ChannelVisibility_CHANNEL_VISIBILITY_UNSPECIFIED
	}
}

func protoState(stored int16) channelv1.ChannelState {
	switch stored {
	case 1:
		return channelv1.ChannelState_CHANNEL_STATE_ACTIVE
	case 2:
		return channelv1.ChannelState_CHANNEL_STATE_ARCHIVED
	case 3:
		return channelv1.ChannelState_CHANNEL_STATE_PENDING_DELETION
	default:
		return channelv1.ChannelState_CHANNEL_STATE_UNSPECIFIED
	}
}
