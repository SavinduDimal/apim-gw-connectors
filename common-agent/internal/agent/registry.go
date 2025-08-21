/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package agent

import (
	"fmt"

	apkAgent "github.com/wso2-extensions/apim-gw-connectors/apk/gateway-connector"
	"github.com/wso2-extensions/apim-gw-connectors/common-agent/pkg/agent"
	// kongAgent "github.com/wso2-extensions/apim-gw-connectors/kong/gateway-connector"
)

// agentRegistry is a registry that holds different gateway agents.
type agentRegistry struct {
	agents map[string]agent.Agent
}

// RegisterAgent adds a new agent to the registry.
func (ar *agentRegistry) RegisterAgent(name string, agent agent.Agent) {
	ar.agents[name] = agent
}

// GetAgent retrieves an agent from the registry by name.
func (ar *agentRegistry) GetAgent(name string) (agent.Agent, error) {
	if agent, ok := ar.agents[name]; ok {
		return agent, nil
	}
	return nil, fmt.Errorf("Agent not found in registry")
}

// An instance of the AgentRegistry.
var agentReg = &agentRegistry{
	agents: make(map[string]agent.Agent), // Initialize the agent map.
}

// init function registers the default gateway agents when the package is initialized.
func init() {
	agentReg.RegisterAgent("apk", &apkAgent.Agent{})
	// agentReg.RegisterAgent("kong", &kongAgent.Agent{})
}
