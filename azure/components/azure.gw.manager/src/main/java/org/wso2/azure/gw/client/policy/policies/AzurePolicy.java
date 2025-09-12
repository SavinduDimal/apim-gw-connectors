/*
 * Copyright (c) 2025 WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package org.wso2.azure.gw.client.policy.policies;

import org.w3c.dom.Element;
import org.wso2.azure.gw.client.policy.AzurePolicyType;
import org.wso2.carbon.apimgt.api.APIManagementException;
import javax.xml.parsers.DocumentBuilder;

/**
 * Abstract class representing an Azure policy.
 */
public abstract class AzurePolicy {
    private AzurePolicyType type;
    private Element root = null;

    public AzurePolicyType getType() {
        return this.type;
    }

    public void setType(AzurePolicyType type) {
        this.type = type;
    }

    public Element getRoot() {
        return this.root;
    }

    public void setRoot(Element root) {
        this.root = root;
    }

    public abstract void processDocument(DocumentBuilder documentBuilder) throws APIManagementException;
}
