package chat

import (
	"context"

	"admin_back_go/internal/apperror"
)

type HTTPService interface {
	ListConversations(ctx context.Context, identity Identity, query ConversationListQuery) (*ConversationListResponse, *apperror.Error)
	CreatePrivate(ctx context.Context, identity Identity, input CreatePrivateInput) (*CreatePrivateResponse, *apperror.Error)
	ListMessages(ctx context.Context, identity Identity, query MessageListQuery) (*MessageListResponse, *apperror.Error)
	SendMessage(ctx context.Context, identity Identity, input SendMessageInput) (*SendMessageResponse, *apperror.Error)
	MarkRead(ctx context.Context, identity Identity, input MarkReadInput) *apperror.Error
	ListContacts(ctx context.Context, identity Identity) (*ContactListResponse, *apperror.Error)
	AddContact(ctx context.Context, identity Identity, input ContactInput) *apperror.Error
	ConfirmContact(ctx context.Context, identity Identity, input ContactInput) *apperror.Error
	DeleteContact(ctx context.Context, identity Identity, input ContactInput) *apperror.Error
	DeleteConversation(ctx context.Context, identity Identity, conversationID int64) *apperror.Error
	TogglePin(ctx context.Context, identity Identity, conversationID int64) *apperror.Error
}
