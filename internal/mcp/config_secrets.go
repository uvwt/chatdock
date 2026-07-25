package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

const mcpTokenReferenceField = "_chatdock_token_ref"

func RedactConfigTokens(content string) (string, error) {
	root, servers, err := parseMCPConfigDocument(content)
	if err != nil {
		return "", err
	}
	for serverName, rawServer := range servers {
		server, auth, err := parseMCPServerAuth(serverName, rawServer)
		if err != nil {
			return "", err
		}
		if auth == nil {
			continue
		}
		token, err := rawJSONString(auth["token"])
		if err != nil {
			return "", fmt.Errorf("mcp server %s token: %w", serverName, err)
		}
		delete(auth, "token")
		delete(auth, mcpTokenReferenceField)
		if strings.TrimSpace(token) != "" {
			auth[mcpTokenReferenceField] = mustMarshalJSON(serverName)
		}
		if err := writeMCPServerAuth(servers, serverName, server, auth); err != nil {
			return "", err
		}
	}
	return marshalMCPConfigDocument(root, servers)
}

func MergeConfigTokens(currentContent, submittedContent string) (string, error) {
	_, currentServers, err := parseMCPConfigDocument(currentContent)
	if err != nil {
		return "", fmt.Errorf("read current mcp config: %w", err)
	}
	currentTokens := make(map[string]json.RawMessage, len(currentServers))
	for serverName, rawServer := range currentServers {
		_, auth, err := parseMCPServerAuth(serverName, rawServer)
		if err != nil {
			return "", err
		}
		if auth == nil {
			continue
		}
		token, err := rawJSONString(auth["token"])
		if err != nil {
			return "", fmt.Errorf("mcp server %s token: %w", serverName, err)
		}
		if strings.TrimSpace(token) != "" {
			currentTokens[serverName] = mustMarshalJSON(token)
		}
	}

	root, submittedServers, err := parseMCPConfigDocument(submittedContent)
	if err != nil {
		return "", err
	}
	usedReferences := map[string]string{}
	for serverName, rawServer := range submittedServers {
		server, auth, err := parseMCPServerAuth(serverName, rawServer)
		if err != nil {
			return "", err
		}
		if auth == nil {
			continue
		}

		// 浏览器只持有一次性引用，不持有旧秘密。新值优先；留空且带引用时恢复旧值；
		// 没有新值也没有引用则代表用户明确清除 Token。
		token, err := rawJSONString(auth["token"])
		if err != nil {
			return "", fmt.Errorf("mcp server %s token: %w", serverName, err)
		}
		reference, err := rawJSONString(auth[mcpTokenReferenceField])
		if err != nil {
			return "", fmt.Errorf("mcp server %s token reference: %w", serverName, err)
		}
		if strings.TrimSpace(token) != "" {
			normalizedToken := normalizeBearerToken(token)
			if normalizedToken == "" {
				return "", fmt.Errorf("mcp server %s token is empty after removing the Bearer prefix", serverName)
			}
			auth["token"] = mustMarshalJSON(normalizedToken)
			delete(auth, mcpTokenReferenceField)
		} else if strings.TrimSpace(reference) != "" {
			reference = strings.TrimSpace(reference)
			if previousServer, exists := usedReferences[reference]; exists {
				return "", fmt.Errorf("mcp token reference %q is already used by server %s", reference, previousServer)
			}
			token, exists := currentTokens[reference]
			if !exists {
				return "", fmt.Errorf("mcp server %s references an unavailable saved token", serverName)
			}
			usedReferences[reference] = serverName
			auth["token"] = token
			delete(auth, mcpTokenReferenceField)
		} else {
			delete(auth, "token")
			delete(auth, mcpTokenReferenceField)
		}
		if err := writeMCPServerAuth(submittedServers, serverName, server, auth); err != nil {
			return "", err
		}
	}
	return marshalMCPConfigDocument(root, submittedServers)
}

func parseMCPConfigDocument(content string) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return nil, nil, fmt.Errorf("invalid mcp config json: %w", err)
	}
	if root == nil {
		return nil, nil, fmt.Errorf("invalid mcp config json: root must be an object")
	}
	servers := map[string]json.RawMessage{}
	if rawServers, ok := root["servers"]; ok && len(rawServers) > 0 && string(rawServers) != "null" {
		if err := json.Unmarshal(rawServers, &servers); err != nil {
			return nil, nil, fmt.Errorf("invalid mcp servers: %w", err)
		}
	}
	return root, servers, nil
}

func parseMCPServerAuth(serverName string, rawServer json.RawMessage) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	var server map[string]json.RawMessage
	if err := json.Unmarshal(rawServer, &server); err != nil {
		return nil, nil, fmt.Errorf("invalid mcp server %s: %w", serverName, err)
	}
	rawAuth, ok := server["auth"]
	if !ok || len(rawAuth) == 0 || string(rawAuth) == "null" {
		return server, nil, nil
	}
	var auth map[string]json.RawMessage
	if err := json.Unmarshal(rawAuth, &auth); err != nil {
		return nil, nil, fmt.Errorf("invalid mcp server %s auth: %w", serverName, err)
	}
	return server, auth, nil
}

func writeMCPServerAuth(servers map[string]json.RawMessage, serverName string, server, auth map[string]json.RawMessage) error {
	rawAuth, err := json.Marshal(auth)
	if err != nil {
		return fmt.Errorf("marshal mcp server %s auth: %w", serverName, err)
	}
	server["auth"] = rawAuth
	rawServer, err := json.Marshal(server)
	if err != nil {
		return fmt.Errorf("marshal mcp server %s: %w", serverName, err)
	}
	servers[serverName] = rawServer
	return nil
}

func marshalMCPConfigDocument(root, servers map[string]json.RawMessage) (string, error) {
	rawServers, err := json.Marshal(servers)
	if err != nil {
		return "", fmt.Errorf("marshal mcp servers: %w", err)
	}
	root["servers"] = rawServers
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	return string(data) + "\n", nil
}

func rawJSONString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("must be a string")
	}
	return value, nil
}

func mustMarshalJSON(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
