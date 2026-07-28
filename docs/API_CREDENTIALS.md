# API 凭据权限建议

## Cloudflare

应用只支持 API Token，不支持 Global API Key。

建议创建最小权限 Token：

- Zone → Zone → Read
- Zone → DNS → Edit
- Zone Resources → 只选择需要管理的 Zone，或选择账户下全部 Zone

应用使用 Token 验证接口检查有效性，并通过 `/zones` 与 `/dns_records` 管理数据。

## 腾讯云 DNSPod

使用腾讯云访问密钥 `SecretId + SecretKey`，调用 DNSPod API 2021-03-23。

所需动作：

- `dnspod:DescribeDomainList`
- `dnspod:DescribeRecordList`
- `dnspod:DescribeRecord`
- `dnspod:CreateRecord`
- `dnspod:ModifyRecord`
- `dnspod:DeleteRecord`
- `dnspod:ModifyRecordStatus`

可以使用腾讯云预设 DNSPod 管理策略，也可以按上述动作创建更小权限的自定义策略。建议使用专门的子用户密钥，不要使用主账号永久密钥。

## 本地保存方式

凭据先序列化，再使用随机 nonce 的 AES-256-GCM 加密。数据库只保存密文和用于识别的脱敏提示；主密钥保存在同一 FNOS 设备的独立权限文件中。

编辑凭据时，密钥输入框留空会保留现有密文。删除凭据会立即删除其域名和记录缓存，但对应审计日志仍保留至六个月期限。

