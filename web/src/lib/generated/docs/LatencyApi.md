# LatencyApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getDelay**](LatencyApi.md#getdelay) | **GET** /delay/{seconds} | Delay the response by a given number of seconds |



## getDelay

> DelayResponse getDelay(seconds)

Delay the response by a given number of seconds

Waits for the requested number of seconds before responding. The delay is capped at 10 seconds; larger values are clamped to the cap. Negative values yield a 400 response. The wait respects request cancellation and the server\&#39;s request timeout. 

### Example

```ts
import {
  Configuration,
  LatencyApi,
} from '';
import type { GetDelayRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new LatencyApi();

  const body = {
    // number | The number of seconds to delay (0-10).
    seconds: 1,
  } satisfies GetDelayRequest;

  try {
    const data = await api.getDelay(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **seconds** | `number` | The number of seconds to delay (0-10). | [Defaults to `undefined`] |

### Return type

[**DelayResponse**](DelayResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The response after the requested delay. |  -  |
| **400** | The requested delay is negative. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

