INSERT IGNORE INTO `mail_templates` (
  `scene`, `name`, `subject`, `tencent_template_id`,
  `variables_json`, `sample_variables_json`, `status`, `is_del`
) VALUES
('login', '邮箱验证码登录', '验证码登录', 47941, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2),
('forget', '找回密码', '找回密码验证码', 47942, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2),
('bind_email', '绑定/换绑邮箱', '绑定邮箱验证码', 47943, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2),
('change_password', '验证码改密', '修改密码验证码', 47944, JSON_ARRAY('code', 'ttl_minutes'), JSON_OBJECT('code', '123456', 'ttl_minutes', '5'), 1, 2);
