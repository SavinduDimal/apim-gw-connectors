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

import org.w3c.dom.Document;
import org.w3c.dom.Element;
import org.wso2.azure.gw.client.AzureConstants;
import org.wso2.azure.gw.client.AzureGatewayConfiguration;
import org.wso2.azure.gw.client.policy.AzurePolicyType;
import org.wso2.carbon.apimgt.api.APIManagementException;
import org.xml.sax.SAXException;
import java.io.IOException;
import java.io.InputStream;
import javax.xml.parsers.DocumentBuilder;

/**
 * This class manages the Azure API Management Set Header policy.
 */
public class AzureRateLimitPolicy extends AzurePolicy {

    String calls;
    String renewalPeriod;
    String retryAfterHeaderName;
    String retryAfterVariableName;
    String remainingCallsHeaderName;
    String remainingCallsVariableName;
    String totalCallsHeaderName;

    public AzureRateLimitPolicy(String calls, String renewalPeriod, String retryAfterHeaderName,
                                String retryAfterVariableName, String remainingCallsHeaderName,
                                String remainingCallsVariableName, String totalCallsHeaderName)
            throws APIManagementException {
        this.setType(AzurePolicyType.RATE_LIMIT);
        this.calls = calls;
        this.renewalPeriod = renewalPeriod;
        this.retryAfterHeaderName = retryAfterHeaderName;
        this.retryAfterVariableName = retryAfterVariableName;
        this.remainingCallsHeaderName = remainingCallsHeaderName;
        this.remainingCallsVariableName = remainingCallsVariableName;
        this.totalCallsHeaderName = totalCallsHeaderName;
    }

    @Override
    public void processDocument(DocumentBuilder documentBuilder) throws APIManagementException {
        setAttributesValuesToPolicy(documentBuilder);
    }

    private void readPolicyFile(DocumentBuilder documentBuilder) throws APIManagementException {
        if (this.getRoot() != null) {
            return;
        }
        Document policyDocument = null;
        try (InputStream inputStream = AzureGatewayConfiguration.class.getClassLoader()
                .getResourceAsStream(AzureConstants.AZURE_RATE_LIMIT_POLICY_FILENAME)) {
            if (inputStream == null) {
                throw new APIManagementException("Rate Limit Policy file not found");
            }
            policyDocument = documentBuilder.parse(inputStream);
        } catch (IOException e) {
            throw new APIManagementException("Error reading Rate Limit policy file", e);
        } catch (SAXException e) {
            throw new APIManagementException("Error parsing Rate Limit policy file", e);
        }
        this.setRoot(policyDocument.getDocumentElement());
    }

    private void setAttributesValuesToPolicy(DocumentBuilder documentBuilder) throws APIManagementException {
        readPolicyFile(documentBuilder);

        Element rateLimitElement = this.getRoot();
        if (!"rate-limit".equals(rateLimitElement.getTagName())) {
            throw new APIManagementException("Rate limit policy does not contain rate-limit element");
        }
        rateLimitElement.setAttribute("calls", this.calls);
        rateLimitElement.setAttribute("renewal-period", this.renewalPeriod);

        if (this.retryAfterHeaderName != null) {
            rateLimitElement.setAttribute("retry-after-header-name", this.retryAfterHeaderName);
        }
        if (this.retryAfterVariableName != null) {
            rateLimitElement.setAttribute("retry-after-variable-name", this.retryAfterVariableName);
        }
        if (this.remainingCallsHeaderName != null) {
            rateLimitElement.setAttribute("remaining-calls-header-name", this.remainingCallsHeaderName);
        }
        if (this.remainingCallsVariableName != null) {
            rateLimitElement.setAttribute("remaining-calls-variable-name", this.remainingCallsVariableName);
        }
        if (this.totalCallsHeaderName != null) {
            rateLimitElement.setAttribute("total-calls-header-name", this.totalCallsHeaderName);
        }
    }
}
