package org.wso2.kong.client;

/**
 * Custom exception class for handling errors from Kong Gateway.
 */

public class KongGatewayException extends Exception {
private String statusCode;
private String message;

    public KongGatewayException(String statusCode, String message) {
        super(message);
        this.statusCode = statusCode;
        this.message = message;
    }

    public KongGatewayException(String message, Exception cause) {
        super(message, cause);
    }

    public String getStatusCode() {
        return statusCode;
    }

    public String getMessage() {
        return message;
    }

    @Override
    public String toString() {
        return "KongGatewayException{" +
                "statusCode='" + statusCode + '\'' +
                ", message='" + message + '\'' +
                '}';
    }
}
