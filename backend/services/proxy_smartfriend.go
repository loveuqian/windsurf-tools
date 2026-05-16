// F7-REMOVAL: 整文件删除（仅作者自用功能；发布前彻底移除）
// 关联清理：proxy.go / openai_relay.go / relay_anthropic.go / chat_proto.go 中所有
// smartFriendEnabled 字段引用 + SetSmartFriendEnabled / GetSmartFriendEnabled 调用，
// app_settings.go syncSmartFriendConfig，app_switch.go shouldBypassQuotaCheck 等。
// 详见仓库根 docs/F7-REMOVAL.md。
package services

// PatchF7ToSmartFriend rewrites the top-level F7 varint in a Connect-framed
// protobuf body from CASCADE(5) to SMART_FRIEND(13). Returns the patched body
// and true if a patch was applied.
func PatchF7ToSmartFriend(body []byte) ([]byte, bool) {
	raw, etype := decompressBody(body)
	if len(raw) == 0 {
		return body, false
	}
	fields := parseProtobuf(raw)
	patched := false
	for i := range fields {
		if fields[i].FieldNum == 7 && fields[i].WireType == 0 && fields[i].Varint != 13 {
			fields[i].Varint = 13
			patched = true
		}
	}
	if !patched {
		return body, false
	}
	return recompressBody(serializeProtobuf(fields), etype), true
}

// SetSmartFriendEnabled toggles the SmartFriend F7 patching (thread-safe).
func (p *MitmProxy) SetSmartFriendEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.smartFriendEnabled = enabled
}

// GetSmartFriendEnabled returns the current SmartFriend state (thread-safe).
func (p *MitmProxy) GetSmartFriendEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.smartFriendEnabled
}
