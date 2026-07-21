package admin

import (
	"fmt"
	"reflect"

	aiagent "admin_back_go/internal/module/ai/agent"
	aichat "admin_back_go/internal/module/ai/chat"
	aiconversation "admin_back_go/internal/module/ai/conversation"
	aiknowledge "admin_back_go/internal/module/ai/knowledge"
	aimessage "admin_back_go/internal/module/ai/message"
	aiprovider "admin_back_go/internal/module/ai/provider"
	airun "admin_back_go/internal/module/ai/run"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/module/auth"
	authplatformadmin "admin_back_go/internal/module/auth_platform/transport/admin"
	crontask "admin_back_go/internal/module/crontask"
	exporttask "admin_back_go/internal/module/export"
	mailadmin "admin_back_go/internal/module/mail/transport/admin"
	notification "admin_back_go/internal/module/notification"
	notificationtask "admin_back_go/internal/module/notification/task"
	operationlogadmin "admin_back_go/internal/module/operationlog/transport/admin"
	"admin_back_go/internal/module/payment"
	walletadmin "admin_back_go/internal/module/payment/wallet/transport/admin"
	permissionadmin "admin_back_go/internal/module/permission/transport/admin"
	queuemonitoradmin "admin_back_go/internal/module/queuemonitor/transport/admin"
	roleadmin "admin_back_go/internal/module/role/transport/admin"
	smsadmin "admin_back_go/internal/module/sms/transport/admin"
	systemlogadmin "admin_back_go/internal/module/systemlog/transport/admin"
	systemsettingadmin "admin_back_go/internal/module/systemsetting/transport/admin"
	uploadconfigadmin "admin_back_go/internal/module/uploadconfig/transport/admin"
	uploadtokenadmin "admin_back_go/internal/module/uploadtoken/transport/admin"
	"admin_back_go/internal/module/user"
)

type Graph struct {
	Identity       IdentityGraph
	System         SystemGraph
	Communications CommunicationsGraph
	Commerce       CommerceGraph
	AI             AIGraph
}

type IdentityGraph struct {
	Auth          auth.SessionService
	Captcha       auth.CaptchaHTTPService
	Users         user.HTTPService
	Permissions   permissionadmin.ManagementService
	Roles         roleadmin.HTTPService
	AuthPlatforms authplatformadmin.HTTPService
	Sessions      auth.SessionAdminHTTPService
	LoginLogs     auth.LoginLogHTTPService
	BrowserGrants *auth.BrowserGrantService
}

type SystemGraph struct {
	CronTasks     crontask.HTTPService
	Exports       exporttask.HTTPService
	OperationLogs operationlogadmin.HTTPService
	QueueMonitor  queuemonitoradmin.HTTPService
	Settings      systemsettingadmin.HTTPService
	Logs          systemlogadmin.HTTPService
}

type CommunicationsGraph struct {
	Notifications     notification.HTTPService
	NotificationTasks notificationtask.HTTPService
	Mail              mailadmin.HTTPService
	SMS               smsadmin.HTTPService
	UploadConfig      uploadconfigadmin.HTTPService
	UploadTokens      uploadtokenadmin.HTTPService
}

type CommerceGraph struct {
	Payment payment.HTTPService
	Wallet  walletadmin.HTTPService
}

type AIGraph struct {
	Agents        aiagent.HTTPService
	Chat          aichat.HTTPService
	Conversations aiconversation.HTTPService
	Knowledge     aiknowledge.HTTPService
	Messages      aimessage.HTTPService
	Providers     aiprovider.HTTPService
	Runs          airun.HTTPService
	Tools         aitool.HTTPService
}

func (g Graph) Validate() error {
	required := []struct {
		name  string
		value any
	}{
		{name: "identity.auth", value: g.Identity.Auth},
		{name: "identity.captcha", value: g.Identity.Captcha},
		{name: "identity.users", value: g.Identity.Users},
		{name: "identity.permissions", value: g.Identity.Permissions},
		{name: "identity.roles", value: g.Identity.Roles},
		{name: "identity.auth_platforms", value: g.Identity.AuthPlatforms},
		{name: "identity.sessions", value: g.Identity.Sessions},
		{name: "identity.login_logs", value: g.Identity.LoginLogs},
		{name: "identity.browser_grants", value: g.Identity.BrowserGrants},
		{name: "system.cron_tasks", value: g.System.CronTasks},
		{name: "system.exports", value: g.System.Exports},
		{name: "system.operation_logs", value: g.System.OperationLogs},
		{name: "system.queue_monitor", value: g.System.QueueMonitor},
		{name: "system.settings", value: g.System.Settings},
		{name: "system.logs", value: g.System.Logs},
		{name: "communications.notifications", value: g.Communications.Notifications},
		{name: "communications.notification_tasks", value: g.Communications.NotificationTasks},
		{name: "communications.mail", value: g.Communications.Mail},
		{name: "communications.sms", value: g.Communications.SMS},
		{name: "communications.upload_config", value: g.Communications.UploadConfig},
		{name: "communications.upload_tokens", value: g.Communications.UploadTokens},
		{name: "commerce.payment", value: g.Commerce.Payment},
		{name: "commerce.wallet", value: g.Commerce.Wallet},
		{name: "ai.agents", value: g.AI.Agents},
		{name: "ai.chat", value: g.AI.Chat},
		{name: "ai.conversations", value: g.AI.Conversations},
		{name: "ai.knowledge", value: g.AI.Knowledge},
		{name: "ai.messages", value: g.AI.Messages},
		{name: "ai.providers", value: g.AI.Providers},
		{name: "ai.runs", value: g.AI.Runs},
		{name: "ai.tools", value: g.AI.Tools},
	}
	for _, capability := range required {
		if isNilCapability(capability.value) {
			return fmt.Errorf("admin capability %s is required", capability.name)
		}
	}
	return nil
}

func isNilCapability(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
