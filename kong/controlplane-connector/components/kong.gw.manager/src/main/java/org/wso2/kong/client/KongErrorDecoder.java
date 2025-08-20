package org.wso2.kong.client;

import feign.Response;
import feign.codec.ErrorDecoder;
import org.apache.commons.io.IOUtils;
import org.apache.commons.logging.Log;
import org.apache.commons.logging.LogFactory;
import java.io.IOException;

/**
 * Custom Error Decoder for handling errors from Kong Gateway.
 * This class implements the ErrorDecoder interface to decode error responses
 * from the Kong Gateway and throw a custom exception.
 */
public class KongErrorDecoder implements ErrorDecoder {

    private static final Log log = LogFactory.getLog(KongErrorDecoder.class);

    @Override
    public KongGatewayException decode(String s, Response response) {
        try {
            String responseStr = IOUtils.toString(response.body().asInputStream(), "UTF-8");
            return new KongGatewayException(s, responseStr);
        } catch (IOException e) {
            return new KongGatewayException("Error reading response body: " + e.getMessage(), e);
        }

    }
}
