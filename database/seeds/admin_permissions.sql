START TRANSACTION;

CREATE TEMPORARY TABLE `_admin_permission_seed_guard` (
  `value` tinyint NOT NULL PRIMARY KEY
);

INSERT INTO `_admin_permission_seed_guard` (`value`) VALUES (1);

INSERT INTO `_admin_permission_seed_guard` (`value`)
SELECT 1 FROM `permissions` LIMIT 1;

CREATE TEMPORARY TABLE `_ai_run_permission_seed_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_run_permission_seed_guard`
SELECT IF(
  COUNT(*) = COALESCE(SUM(
    `id` = 920
    AND BINARY `code` = BINARY 'ai_run_list'
    AND `parent_id` = 50
    AND `type` = 3
    AND `status` = 1
    AND `is_del` IN (1, 2)
  ), 0),
  0,
  1
)
FROM `permissions`
WHERE `id` = 920 OR `code` = 'ai_run_list';

DROP TEMPORARY TABLE `_ai_run_permission_seed_guard`;

CREATE TEMPORARY TABLE `_ai_official_model_permission_seed_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_ai_official_model_permission_seed_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `permissions`
WHERE `id` IN (921, 922)
   OR `code` IN ('ai_official_model_list', 'ai_official_model_price_sync');

DROP TEMPORARY TABLE `_ai_official_model_permission_seed_guard`;

DROP TEMPORARY TABLE `_admin_permission_seed_guard`;

INSERT INTO `permissions` (
  `id`, `name`, `path`, `icon`, `parent_id`, `component`, `platform`,
  `type`, `sort`, `code`, `i18n_key`, `show_menu`, `status`, `is_del`
) VALUES
(1, '用户', '', 'tabler:calendar', 0, '', 'admin', 1, 1, NULL, 'menu.user', 1, 1, 2),
(2, '权限管理', '', 'Lock', 0, NULL, 'admin', 1, 2, NULL, 'menu.permissionMgmt', 1, 1, 2),
(3, '系统管理', '', 'List', 0, NULL, 'admin', 1, 3, NULL, 'menu.system', 1, 1, 2),
(4, '组件演示', '', 'Menu', 0, '', 'admin', 1, 4, NULL, 'menu.component', 1, 1, 2),
(5, 'AI助手', '', 'HelpFilled', 0, NULL, 'admin', 1, 5, NULL, 'menu.ai', 1, 1, 2),
(7, '用户管理', '/user/userManager', '', 1, 'user/userManager', 'admin', 2, 1, NULL, 'menu.user_userManager', 1, 1, 2),
(8, '登录日志', '/user/usersLoginLog', '', 1, 'user/usersLoginLog', 'admin', 2, 2, NULL, 'menu.user_usersLoginLog', 1, 1, 2),
(9, '编辑', '', '', 7, NULL, 'admin', 3, 1, 'user_userManager_edit', '', 2, 1, 2),
(10, '删除', '', '', 7, NULL, 'admin', 3, 2, 'user_userManager_del', '', 2, 1, 2),
(11, '踢下线', '', '', 7, NULL, 'admin', 3, 3, 'user_userManager_kick', '', 2, 1, 2),
(12, '后台菜单管理', '/permission/permission', '', 2, 'permission/permission', 'admin', 2, 1, NULL, 'menu.permission_permission', 1, 1, 2),
(13, '角色管理', '/permission/role', '', 2, 'permission/role', 'admin', 2, 3, NULL, 'menu.permission_role', 1, 1, 2),
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
(122, '知识库', '/ai/knowledge', 'Collection', 5, 'ai/knowledge', 'admin', 2, 3, NULL, 'menu.ai_knowledge', 1, 1, 2),
(123, '知识库召回测试', '', '', 122, NULL, 'admin', 3, 9, 'ai_knowledge_retrieval_test', '', 2, 1, 2),
(124, '知识库重建索引', '', '', 122, NULL, 'admin', 3, 8, 'ai_knowledge_reindex', '', 2, 1, 2),
(125, '知识库文档删除', '', '', 122, NULL, 'admin', 3, 9, 'ai_knowledge_document_del', '', 2, 1, 2),
(126, '知识库文档编辑', '', '', 122, NULL, 'admin', 3, 6, 'ai_knowledge_document_edit', '', 2, 1, 2),
(127, '知识库文档新增', '', '', 122, NULL, 'admin', 3, 6, 'ai_knowledge_document_add', '', 2, 1, 2),
(128, '知识库状态', '', '', 122, NULL, 'admin', 3, 4, 'ai_knowledge_status', '', 2, 1, 2),
(129, '知识库删除', '', '', 122, NULL, 'admin', 3, 5, 'ai_knowledge_del', '', 2, 1, 2),
(130, '知识库编辑', '', '', 122, NULL, 'admin', 3, 2, 'ai_knowledge_edit', '', 2, 1, 2),
(131, '知识库新增', '', '', 122, NULL, 'admin', 3, 1, 'ai_knowledge_add', '', 2, 1, 2),
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
(413, '新增智能体绑定', '', '', 401, NULL, 'admin', 3, 6, 'ai_agent_binding_add', '', 2, 1, 2),
(415, '知识库文档状态', '', '', 122, NULL, 'admin', 3, 7, 'ai_knowledge_document_status', '', 2, 1, 2),
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
(922, '同步官方模型价格', '', '', 921, NULL, 'admin', 3, 1, 'ai_official_model_price_sync', '', 2, 1, 2);

COMMIT;
