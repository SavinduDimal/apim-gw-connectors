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
import org.wso2.azure.gw.client.policy.AzurePolicyUtil;
import org.wso2.carbon.apimgt.api.APIManagementException;
import org.xml.sax.SAXException;
import java.io.IOException;
import java.io.InputStream;
import javax.xml.parsers.DocumentBuilder;

/**
 * This class manages the Azure API Management Set Header policy.
 */
public class AzureSetHeaderPolicy extends AzurePolicy {

    private String headerName;
    private String headerValue;
    private String existsAction;

    public AzureSetHeaderPolicy(String headerName, String headerValue, String existsAction)
            throws APIManagementException {
        this.setType(AzurePolicyType.SET_HEADER);
        this.headerName = headerName;
        this.headerValue = headerValue;
        this.existsAction = existsAction;
    }

    @Override
    public void processDocument(DocumentBuilder documentBuilder) throws APIManagementException {
        setAttributesValuesToPolicy(documentBuilder);
    }

    private void readSetHeaderPolicy(DocumentBuilder documentBuilder) throws APIManagementException {
        if (this.getRoot() != null) {
            return;
        }
        Document policyDocument = null;
        try (InputStream inputStream = AzureGatewayConfiguration.class.getClassLoader()
                .getResourceAsStream(AzureConstants.AZURE_SET_HEADER_POLICY_FILENAME)) {
            if (inputStream == null) {
                throw new APIManagementException("Set Header Policy file not found");
            }
            policyDocument = documentBuilder.parse(inputStream);
        } catch (IOException e) {
            throw new APIManagementException("Error reading Set Header policy file", e);
        } catch (SAXException e) {
            throw new APIManagementException("Error parsing Set Header policy file", e);
        }
        this.setRoot(policyDocument.getDocumentElement());
    }

    private void setAttributesValuesToPolicy(DocumentBuilder documentBuilder) throws APIManagementException {
        readSetHeaderPolicy(documentBuilder);

        Element setHeaderElement = this.getRoot();
        if (!"set-header".equals(setHeaderElement.getTagName())) {
            throw new APIManagementException("Set Header policy does not contain set-header element");
        }
        setHeaderElement.setAttribute("name", this.headerName);
        setHeaderElement.setAttribute("exists-action", this.existsAction);
        Element valueElement = AzurePolicyUtil.firstChildElementByTagName(setHeaderElement, "value");
        if (!"delete".equals(this.existsAction)) {
            if (valueElement == null) {
                throw new APIManagementException("Set Header policy does not contain value element");
            }
            valueElement.setTextContent(this.headerValue);
        } else {
            this.getRoot().removeChild(valueElement);
        }
    }
}
