-- Canonical local initialization data. Apply only after database/schema.sql to an empty schema.
START TRANSACTION;
INSERT INTO `permissions` (
  `id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`,
  `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`
) VALUES
(1, '用户', '', 'tabler:calendar', 0, '', 'admin', 1, 1, NULL, 'menu.user', 1, 1, 2),
(2, '权限管理', '', 'Lock', 0, NULL, 'admin', 1, 2, NULL, 'menu.permissionMgmt', 1, 1, 2),
(3, '系统管理', '', 'List', 0, NULL, 'admin', 1, 3, NULL, 'menu.system', 1, 1, 2),
(4, '组件演示', '', 'Menu', 0, '', 'admin', 1, 4, NULL, 'menu.component', 1, 1, 2),
(5, 'AI助手', '', 'HelpFilled', 0, NULL, 'admin', 1, 5, NULL, 'menu.ai', 1, 1, 2),
(7, '用户管理', '/user/userManager', '', 1, 'user/userManager', 'admin', 2, 1, 'user_userManager', 'menu.user_userManager', 1, 1, 2),
(8, '登录日志', '/user/usersLoginLog', '', 1, 'user/usersLoginLog', 'admin', 2, 2, NULL, 'menu.user_usersLoginLog', 1, 1, 2),
(9, '编辑', '', '', 7, NULL, 'admin', 3, 1, 'user_userManager_edit', '', 2, 1, 2),
(10, '删除', '', '', 7, NULL, 'admin', 3, 2, 'user_userManager_del', '', 2, 1, 2),
(11, '踢下线', '', '', 7, NULL, 'admin', 3, 3, 'user_userManager_kick', '', 2, 1, 2),
(12, '后台菜单管理', '/permission/permission', '', 2, 'permission/permission', 'admin', 2, 1, NULL, 'menu.permission_permission', 1, 1, 2),
(13, '角色管理', '/permission/role', '', 2, 'permission/role', 'admin', 2, 3, 'permission_role', 'menu.permission_role', 1, 1, 2),
(14, '新增', '', '', 12, NULL, 'admin', 3, 1, 'permission_permission_add', '', 2, 1, 2),
(15, '编辑', '', '', 12, NULL, 'admin', 3, 2, 'permission_permission_edit', '', 2, 1, 2),
(16, '删除', '', '', 12, NULL, 'admin', 3, 3, 'permission_permission_del', '', 2, 1, 2),
(17, '状态变更', '', '', 12, NULL, 'admin', 3, 4, 'permission_permission_status', '', 2, 1, 2),
(18, '新增', '', '', 13, NULL, 'admin', 3, 1, 'permission_role_add', '', 2, 1, 2),
(19, '编辑', '', '', 13, NULL, 'admin', 3, 2, 'permission_role_edit', '', 2, 1, 2),
(20, '删除', '', '', 13, NULL, 'admin', 3, 3, 'permission_role_del', '', 2, 1, 2),
(21, '设置默认', '', '', 13, NULL, 'admin', 3, 4, 'permission_role_setDefault', '', 2, 1, 2),
(22, '通知管理', '/system/notificationTask', '', 3, 'system/notificationTask', 'admin', 2, 1, NULL, 'menu.system_notificationTask', 1, 1, 2),
(23, '上传配置', '/system/uploadConfig', '', 3, 'system/uploadConfig', 'admin', 2, 2, NULL, 'menu.system_uploadConfig', 1, 1, 2),
(24, '系统设置', '/system/setting', '', 3, 'system/setting', 'admin', 2, 3, NULL, 'menu.system_setting', 1, 1, 2),
(26, '驱动新增', '', '', 23, NULL, 'admin', 3, 1, 'system_uploadConfig_driverAdd', '', 2, 1, 2),
(27, '驱动编辑', '', '', 23, NULL, 'admin', 3, 2, 'system_uploadConfig_driverEdit', '', 2, 1, 2),
(28, '驱动删除', '', '', 23, NULL, 'admin', 3, 3, 'system_uploadConfig_driverDel', '', 2, 1, 2),
(29, '规则新增', '', '', 23, NULL, 'admin', 3, 4, 'system_uploadConfig_ruleAdd', '', 2, 1, 2),
(30, '规则编辑', '', '', 23, NULL, 'admin', 3, 5, 'system_uploadConfig_ruleEdit', '', 2, 1, 2),
(31, '规则删除', '', '', 23, NULL, 'admin', 3, 6, 'system_uploadConfig_ruleDel', '', 2, 1, 2),
(32, '设置新增', '', '', 23, NULL, 'admin', 3, 7, 'system_uploadConfig_settingAdd', '', 2, 1, 2),
(33, '设置编辑', '', '', 23, NULL, 'admin', 3, 8, 'system_uploadConfig_settingEdit', '', 2, 1, 2),
(34, '设置删除', '', '', 23, NULL, 'admin', 3, 9, 'system_uploadConfig_settingDel', '', 2, 1, 2),
(35, '设置状态', '', '', 23, NULL, 'admin', 3, 10, 'system_uploadConfig_settingStatus', '', 2, 1, 2),
(36, '新增', '', '', 24, NULL, 'admin', 3, 1, 'system_setting_add', '', 2, 1, 2),
(37, '编辑', '', '', 24, NULL, 'admin', 3, 2, 'system_setting_edit', '', 2, 1, 2),
(38, '删除', '', '', 24, NULL, 'admin', 3, 3, 'system_setting_del', '', 2, 1, 2),
(39, '状态变更', '', '', 24, NULL, 'admin', 3, 4, 'system_setting_status', '', 2, 1, 2),
(40, '上传', '/component/upload', '', 4, 'component/upload', 'admin', 2, 1, NULL, 'menu.component_upload', 1, 1, 2),
(41, '表单', '/component/form', '', 4, 'component/form', 'admin', 2, 2, NULL, 'menu.component_form', 1, 1, 2),
(42, '展示', '/component/display', '', 4, 'component/display', 'admin', 2, 3, NULL, 'menu.component_display', 1, 1, 2),
(43, '特效', '/component/effect', '', 4, 'component/effect', 'admin', 2, 4, NULL, 'menu.component_effect', 1, 1, 2),
(49, 'AI对话', '/ai/chat', '', 5, 'ai/chat', 'admin', 2, 6, NULL, 'menu.ai_chat', 1, 1, 2),
(50, '运行监控', '/ai/runs', '', 5, 'ai/runs', 'admin', 2, 5, NULL, 'menu.ai_runs', 1, 1, 2),
(56, '队列监控', '/system/queueMonitor', '', 3, 'system/queueMonitor', 'admin', 2, 5, 'devTools_queueMonitor_list', 'menu.system_queueMonitor', 1, 1, 2),
(57, '操作日志', '/system/operationLog', '', 3, 'system/operationLog', 'admin', 2, 6, NULL, 'menu.system_operationLog', 1, 1, 2),
(58, '导出任务', '/system/exportTask', '', 3, 'system/exportTask', 'admin', 2, 7, NULL, 'menu.system_exportTask', 1, 1, 2),
(59, '定时任务', '/system/cronTask', '', 3, 'system/cronTask', 'admin', 2, 8, NULL, 'menu.system_cronTask', 1, 1, 2),
(61, '删除', '', '', 57, NULL, 'admin', 3, 1, 'devTools_operationLog_del', '', 2, 1, 2),
(62, '新增', '', '', 59, NULL, 'admin', 3, 1, 'devTools_cronTask_add', '', 2, 1, 2),
(63, '编辑', '', '', 59, NULL, 'admin', 3, 2, 'devTools_cronTask_edit', '', 2, 1, 2),
(64, '删除', '', '', 59, NULL, 'admin', 3, 3, 'devTools_cronTask_del', '', 2, 1, 2),
(65, '状态', '', '', 59, NULL, 'admin', 3, 4, 'devTools_cronTask_status', '', 2, 1, 2),
(66, '日志', '', '', 59, NULL, 'admin', 3, 5, 'devTools_cronTask_logs', '', 2, 1, 2),
(72, '个人资料', '/personal', 'User', 0, 'personal', 'admin', 2, 90, NULL, 'menu.personal', 2, 1, 2),
(80, '下载管理器', '/component/download', '', 4, 'component/download', 'admin', 2, 5, NULL, 'menu.component_download', 1, 1, 2),
(81, '系统日志', '/system/log', '', 3, 'system/log', 'admin', 2, 4, NULL, 'menu.system_log', 1, 1, 2),
(82, '文件列表', '', '', 81, NULL, 'admin', 3, 1, 'system_log_files', '', 2, 1, 2),
(83, '日志内容', '', '', 81, NULL, 'admin', 3, 2, 'system_log_content', '', 2, 1, 2),
(84, '通知中心', '/notification', 'carbon:notification', 0, 'notification', 'admin', 2, 91, NULL, 'menu.notification', 2, 1, 2),
(85, '认证平台', '/permission/authPlatform', '', 2, 'permission/authPlatform', 'admin', 2, 4, NULL, 'menu.permission_authPlatform', 1, 1, 2),
(86, '认证平台新增', '', '', 85, NULL, 'admin', 3, 1, 'permission_authPlatform_add', '', 2, 1, 2),
(87, '认证平台编辑', '', '', 85, NULL, 'admin', 3, 2, 'permission_authPlatform_edit', '', 2, 1, 2),
(88, '认证平台删除', '', '', 85, NULL, 'admin', 3, 3, 'permission_authPlatform_del', '', 2, 1, 2),
(89, '认证平台状态变更', '', '', 85, NULL, 'admin', 3, 4, 'permission_authPlatform_status', '', 2, 1, 2),
(94, 'AI工具管理', '/ai/tools', '', 5, 'ai/tools', 'admin', 2, 4, NULL, 'menu.ai_tools', 1, 1, 2),
(117, '批量编辑', '', '', 7, NULL, 'admin', 3, 4, 'user_userManager_batchEdit', '', 2, 1, 2),
(118, '导出', '', '', 7, NULL, 'admin', 3, 5, 'user_userManager_export', '', 2, 1, 2),
(122, '上下文工程', '/ai/context', 'Collection', 5, 'ai/context', 'admin', 2, 3, NULL, 'menu.ai_context', 1, 1, 2),
(241, '发布通知', '', '', 22, NULL, 'admin', 3, 1, 'system_notificationTask_add', '', 2, 1, 2),
(242, '取消任务', '', '', 22, NULL, 'admin', 3, 2, 'system_notificationTask_cancel', '', 2, 1, 2),
(243, '删除任务', '', '', 22, NULL, 'admin', 3, 3, 'system_notificationTask_del', '', 2, 1, 2),
(400, '供应商配置', '/ai/providers', '', 5, 'ai/providers', 'admin', 2, 1, NULL, 'menu.ai_providers', 1, 1, 2),
(401, '智能体配置', '/ai/agents', '', 5, 'ai/agents', 'admin', 2, 2, NULL, 'menu.ai_agents', 1, 1, 2),
(403, '新增供应商', '', '', 400, NULL, 'admin', 3, 1, 'ai_provider_add', '', 2, 1, 2),
(404, '编辑供应商', '', '', 400, NULL, 'admin', 3, 2, 'ai_provider_edit', '', 2, 1, 2),
(405, '测试连接', '', '', 400, NULL, 'admin', 3, 3, 'ai_provider_test', '', 2, 1, 2),
(406, '供应商状态', '', '', 400, NULL, 'admin', 3, 4, 'ai_provider_status', '', 2, 1, 2),
(407, '删除供应商', '', '', 400, NULL, 'admin', 3, 5, 'ai_provider_del', '', 2, 1, 2),
(408, '新增智能体', '', '', 401, NULL, 'admin', 3, 1, 'ai_agent_add', '', 2, 1, 2),
(409, '编辑智能体', '', '', 401, NULL, 'admin', 3, 2, 'ai_agent_edit', '', 2, 1, 2),
(410, '测试智能体', '', '', 401, NULL, 'admin', 3, 3, 'ai_agent_test', '', 2, 1, 2),
(411, '智能体状态', '', '', 401, NULL, 'admin', 3, 4, 'ai_agent_status', '', 2, 1, 2),
(412, '删除智能体', '', '', 401, NULL, 'admin', 3, 5, 'ai_agent_del', '', 2, 1, 2),
(417, '工具新增', '', '', 94, NULL, 'admin', 3, 1, 'ai_tool_add', '', 2, 1, 2),
(418, '工具编辑', '', '', 94, NULL, 'admin', 3, 2, 'ai_tool_edit', '', 2, 1, 2),
(419, '工具状态', '', '', 94, NULL, 'admin', 3, 3, 'ai_tool_status', '', 2, 1, 2),
(420, '工具删除', '', '', 94, NULL, 'admin', 3, 4, 'ai_tool_del', '', 2, 1, 2),
(437, '支付管理', '/payment', 'CreditCard', 0, '', 'admin', 1, 40, 'payment', 'menu.payment', 1, 1, 2),
(486, 'AI生成', '', '', 94, NULL, 'admin', 3, 5, 'ai_tool_generate', '', 2, 1, 2),
(506, '邮件管理', '/system/mail', 'Message', 3, 'system/mail', 'admin', 2, 90, 'system_mail', 'menu.system_mail', 1, 1, 2),
(507, '编辑邮件配置', '', '', 506, NULL, 'admin', 3, 1, 'system_mail_configEdit', '', 2, 1, 2),
(508, '删除邮件配置', '', '', 506, NULL, 'admin', 3, 2, 'system_mail_configDel', '', 2, 1, 2),
(509, '发送测试邮件', '', '', 506, NULL, 'admin', 3, 3, 'system_mail_test', '', 2, 1, 2),
(510, '新增邮件模板', '', '', 506, NULL, 'admin', 3, 4, 'system_mail_templateAdd', '', 2, 1, 2),
(511, '编辑邮件模板', '', '', 506, NULL, 'admin', 3, 5, 'system_mail_templateEdit', '', 2, 1, 2),
(512, '修改邮件模板状态', '', '', 506, NULL, 'admin', 3, 6, 'system_mail_templateStatus', '', 2, 1, 2),
(513, '删除邮件模板', '', '', 506, NULL, 'admin', 3, 7, 'system_mail_templateDel', '', 2, 1, 2),
(514, '删除邮件日志', '', '', 506, NULL, 'admin', 3, 8, 'system_mail_logDel', '', 2, 1, 2),
(515, '查看邮件日志及验证码', '', '', 506, NULL, 'admin', 3, 9, 'system_mail_logView', '', 2, 1, 2),
(530, '支付配置', '/payment/config', 'CreditCard', 437, 'payment/config', 'admin', 2, 10, 'payment_config_list', 'menu.payment_config', 1, 1, 2),
(531, '新增支付配置', '', '', 530, '', 'admin', 3, 1, 'payment_config_add', '', 2, 1, 2),
(532, '编辑支付配置', '', '', 530, '', 'admin', 3, 2, 'payment_config_edit', '', 2, 1, 2),
(533, '切换支付配置状态', '', '', 530, '', 'admin', 3, 3, 'payment_config_status', '', 2, 1, 2),
(534, '删除支付配置', '', '', 530, '', 'admin', 3, 4, 'payment_config_del', '', 2, 1, 2),
(535, '上传支付宝证书', '', '', 530, '', 'admin', 3, 5, 'payment_config_upload_cert', '', 2, 1, 2),
(536, '测试支付配置', '', '', 530, '', 'admin', 3, 6, 'payment_config_test', '', 2, 1, 2),
(561, '充值收银台', '/payment/recharge', 'WalletFilled', 437, 'payment/recharge', 'admin', 2, 40, 'payment_recharge_list', 'menu.payment_recharge', 2, 1, 2),
(562, '创建充值', '', '', 561, '', 'admin', 3, 1, 'payment_recharge_add', '', 2, 1, 2),
(563, '继续支付', '', '', 561, '', 'admin', 3, 2, 'payment_recharge_pay', '', 2, 1, 2),
(577, '短信管理', '/system/sms', 'ChatDotRound', 3, 'system/sms', 'admin', 2, 91, 'system_sms', 'menu.system_sms', 1, 1, 2),
(578, '编辑短信配置', '', '', 577, NULL, 'admin', 3, 1, 'system_sms_configEdit', '', 2, 1, 2),
(579, '删除短信配置', '', '', 577, NULL, 'admin', 3, 2, 'system_sms_configDel', '', 2, 1, 2),
(580, '发送测试短信', '', '', 577, NULL, 'admin', 3, 3, 'system_sms_test', '', 2, 1, 2),
(581, '新增短信模板', '', '', 577, NULL, 'admin', 3, 4, 'system_sms_templateAdd', '', 2, 1, 2),
(582, '编辑短信模板', '', '', 577, NULL, 'admin', 3, 5, 'system_sms_templateEdit', '', 2, 1, 2),
(583, '修改短信模板状态', '', '', 577, NULL, 'admin', 3, 6, 'system_sms_templateStatus', '', 2, 1, 2),
(584, '删除短信模板', '', '', 577, NULL, 'admin', 3, 7, 'system_sms_templateDel', '', 2, 1, 2),
(585, '删除短信日志', '', '', 577, NULL, 'admin', 3, 8, 'system_sms_logDel', '', 2, 1, 2),
(653, '收支明细', '/payment/ledger', 'Tickets', 437, 'payment/ledger', 'admin', 2, 20, 'payment_ledger_list', 'menu.payment_ledger', 1, 1, 2),
(654, '用户钱包', '/payment/wallets', 'Wallet', 437, 'payment/wallets', 'admin', 2, 30, 'payment_wallet_list', 'menu.payment_wallets', 1, 1, 2),
(656, '我的钱包', '/profile/wallet', 'Wallet', 0, 'profile/wallet', 'admin', 2, 90, 'profile_wallet', 'menu.profile_wallet', 2, 1, 2),
(912, '兑换码管理', '/payment/redeem-codes', 'Ticket', 437, 'payment/redeem-codes', 'admin', 2, 35, 'payment_redeem_code_list', 'menu.payment_redeem_codes', 1, 1, 2),
(913, '批量生成兑换码', '', '', 912, NULL, 'admin', 3, 1, 'payment_redeem_code_generate', '', 2, 1, 2),
(914, '作废兑换码', '', '', 912, NULL, 'admin', 3, 2, 'payment_redeem_code_void', '', 2, 1, 2),
(920, '查看运行记录', '', '', 50, NULL, 'admin', 3, 1, 'ai_run_list', '', 2, 1, 2),
(921, '官方模型', '/ai/official-models', '', 5, 'ai/official-models', 'admin', 2, 7, 'ai_official_model_list', 'menu.ai_official_models', 1, 1, 2),
(922, '同步官方模型价格', '', '', 921, NULL, 'admin', 3, 1, 'ai_official_model_price_sync', '', 2, 1, 2),
(923, '查看上下文工程', '', '', 122, NULL, 'admin', 3, 1, 'ai_context_view', '', 2, 1, 2),
(924, '管理上下文空间', '', '', 122, NULL, 'admin', 3, 2, 'ai_context_manage', '', 2, 1, 2),
(925, '管理上下文文档', '', '', 122, NULL, 'admin', 3, 3, 'ai_context_document_manage', '', 2, 1, 2),
(926, '管理上下文配置', '', '', 122, NULL, 'admin', 3, 4, 'ai_context_profile_manage', '', 2, 1, 2),
(927, '执行上下文评测', '', '', 122, NULL, 'admin', 3, 5, 'ai_context_evaluate', '', 2, 1, 2);
INSERT INTO `roles` (`id`, `name`, `is_default`, `is_del`) VALUES
(1, '普通用户', 1, 2),
(2, '超管', 2, 2);

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT 1, `id`, 2 FROM `permissions` WHERE `id` IN (7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,26,27,28,29,30,31,32,33,34,35,36,37,38,39,49,50,56,57,58,59,61,62,63,64,65,66,72,81,82,83,84,85,86,87,88,89,94,117,118,122,241,242,243,400,401,403,404,405,406,407,408,409,410,411,412,417,418,419,420,486,506,507,508,509,510,511,512,513,514,515,530,531,532,533,534,535,536,561,562,563,577,578,579,580,581,582,583,584,585,653,654,656,923,924,925,926,927) ORDER BY `id`;

INSERT INTO `role_permissions` (`role_id`, `permission_id`, `is_del`)
SELECT 2, `id`, 2 FROM `permissions` WHERE `type` <> 1 ORDER BY `id`;

INSERT INTO `auth_platforms` (
  `id`, `code`, `name`, `login_types`, `captcha_type`, `access_ttl`, `refresh_ttl`,
  `bind_platform`, `bind_device`, `bind_ip`, `max_sessions`, `allow_register`, `status`, `is_del`
) VALUES
(1, 'admin', 'PC后台', JSON_ARRAY('email', 'phone', 'password'), 'slide', 14400, 1209600, 1, 2, 2, 1, 1, 1, 2);

INSERT INTO `system_settings` (
  `id`, `setting_key`, `setting_value`, `value_type`, `remark`, `status`, `is_del`
) VALUES
(1, 'user.default_avatar', 'https://cos.zgm2003.cn/avatars/1769948592140-20.png', 1, '用户注册头像', 1, 2),
(15, 'auth.captcha.ttl_minutes', '2', 2, '验证码有效期分钟数', 1, 2),
(16, 'auth.captcha.slide_padding', '10', 2, '滑块容差像素', 1, 2),
(19, 'upload.token.ttl_minutes', '15', 2, '上传临时凭证有效期分钟数', 1, 2);

INSERT INTO `mail_templates` (
  `id`, `scene`, `name`, `subject`, `tencent_template_id`,
  `variables_json`, `sample_variables_json`, `status`, `is_del`
) VALUES
(1, 'login', '邮箱验证码登录', '验证码登录', 47941, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2),
(2, 'forget', '找回密码', '找回密码验证码', 47942, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2),
(3, 'bind_email', '绑定/换绑邮箱', '绑定邮箱验证码', 47943, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2),
(4, 'change_password', '验证码改密', '修改密码验证码', 47944, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2);

INSERT INTO `cron_task` (
  `id`, `name`, `title`, `description`, `cron`, `cron_readable`, `handler`, `status`, `is_del`
) VALUES
(1, 'ai_run_timeout', 'AI运行超时检测', '检测AI运行超时的任务并处理', '0 * * * * *', '每分钟执行', 'ai:run-timeout:v1', 1, 2),
(3, 'notification_task_scheduler', '通知任务调度器', '通知任务调度器', '0 * * * * *', '每分钟', 'notification:dispatch-due:v1', 1, 2),
(15, 'payment_sync_pending_order', '支付中订单补偿同步', '扫描支付中支付宝订单并补偿同步本地订单/充值/钱包状态', '0 */2 * * * *', '每2分钟', 'payment:sync-pending-order:v1', 1, 2),
(16, 'payment_close_expired_order', '过期支付订单关闭', '扫描过期未支付支付宝订单并关闭本地/支付宝订单', '0 */5 * * * *', '每5分钟', 'payment:close-expired-order:v1', 1, 2),
(17, 'export_cleanup_expired', '清理过期导出任务', '由Worker软删除已过期导出任务', '0 0 * * * *', '每小时', 'export:cleanup-expired:v1', 1, 2),
(19, 'realtime_event_retention_cleanup', '清理过期实时事件', '每日清理超过七天的实时事件并推进用户水位', '0 15 3 * * *', '每天03:15', 'realtime:cleanup-expired:v1', 1, 2);

INSERT INTO `ai_tools` (
  `id`, `name`, `code`, `description`, `parameters_json`, `result_schema_json`,
  `risk_level`, `timeout_ms`, `status`, `is_del`
) VALUES (
  1,
  '查询当前用户量',
  'admin_user_count',
  '查询后台当前用户数量，只返回总数、启用数、禁用数，不返回任何用户个人信息。',
  JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT(), 'additionalProperties', CAST('false' AS JSON)),
  JSON_OBJECT(
    'type', 'object',
    'required', JSON_ARRAY('total_users', 'enabled_users', 'disabled_users'),
    'properties', JSON_OBJECT(
      'total_users', JSON_OBJECT('type', 'integer', 'minimum', 0),
      'enabled_users', JSON_OBJECT('type', 'integer', 'minimum', 0),
      'disabled_users', JSON_OBJECT('type', 'integer', 'minimum', 0)
    ),
    'additionalProperties', CAST('false' AS JSON)
  ),
  'low', 3000, 1, 2
);

INSERT INTO `schema_migrations` (`version`, `checksum_sha256`)
VALUES ('202608130001', 'b4884de66b62700e47fe2481769012ef4f28ddeffaf7fb6e39e19bbe5fea4033');

COMMIT;
