# InspectApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getEcho**](InspectApi.md#getecho) | **GET** /echo | Echo the incoming request |
| [**getHeaders**](InspectApi.md#getheaders) | **GET** /headers | Echo the request headers |
| [**getIp**](InspectApi.md#getip) | **GET** /ip | Return the caller\&#39;s IP address |
| [**getUserAgent**](InspectApi.md#getuseragent) | **GET** /user-agent | Echo the User-Agent header |
| [**getUuid**](InspectApi.md#getuuid) | **GET** /uuid | Generate a random UUID |



## getEcho

> EchoResponse getEcho()

Echo the incoming request

Returns details of the incoming request: HTTP method, path, query parameters, headers, and origin IP address. 

### Example

```ts
import {
  Configuration,
  InspectApi,
} from '';
import type { GetEchoRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new InspectApi();

  try {
    const data = await api.getEcho();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**EchoResponse**](EchoResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | A reflection of the incoming request. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getHeaders

> HeadersResponse getHeaders()

Echo the request headers

Returns the HTTP headers received with the request.

### Example

```ts
import {
  Configuration,
  InspectApi,
} from '';
import type { GetHeadersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new InspectApi();

  try {
    const data = await api.getHeaders();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**HeadersResponse**](HeadersResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The request headers. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getIp

> IpResponse getIp()

Return the caller\&#39;s IP address

Returns the origin IP address of the incoming request.

### Example

```ts
import {
  Configuration,
  InspectApi,
} from '';
import type { GetIpRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new InspectApi();

  try {
    const data = await api.getIp();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**IpResponse**](IpResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The caller\&#39;s origin IP address. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getUserAgent

> UserAgentResponse getUserAgent()

Echo the User-Agent header

Returns the User-Agent header received with the request.

### Example

```ts
import {
  Configuration,
  InspectApi,
} from '';
import type { GetUserAgentRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new InspectApi();

  try {
    const data = await api.getUserAgent();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**UserAgentResponse**](UserAgentResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The request User-Agent. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getUuid

> UuidResponse getUuid()

Generate a random UUID

Returns a randomly generated version 4 UUID.

### Example

```ts
import {
  Configuration,
  InspectApi,
} from '';
import type { GetUuidRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new InspectApi();

  try {
    const data = await api.getUuid();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**UuidResponse**](UuidResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | A random UUIDv4. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

