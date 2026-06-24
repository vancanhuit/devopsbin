# RateLimitApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getRatelimit**](RateLimitApi.md#getratelimit) | **GET** /ratelimit | Probe the per-IP rate limiter |



## getRatelimit

> RateLimitResponse getRatelimit()

Probe the per-IP rate limiter

Counts this request against a Redis-backed fixed window keyed by the caller\&#39;s client IP. While the request is within the limit the endpoint returns 200 with the remaining allowance; once the limit is exceeded it returns 429 until the window resets. Every response includes the RateLimit-Limit, RateLimit-Remaining, and RateLimit-Reset headers, and the 429 also includes Retry-After. Send the request repeatedly to cross the threshold. 

### Example

```ts
import {
  Configuration,
  RateLimitApi,
} from '';
import type { GetRatelimitRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new RateLimitApi();

  try {
    const data = await api.getRatelimit();
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

[**RateLimitResponse**](RateLimitResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The request was within the limit for the current window. |  * RateLimit-Limit - The maximum number of requests allowed per window. <br>  * RateLimit-Remaining - The number of requests remaining in the current window. <br>  * RateLimit-Reset - Seconds until the current window resets. <br>  |
| **429** | The rate limit was exceeded for the current window. Retry after the period indicated by the Retry-After header.  |  * RateLimit-Limit - The maximum number of requests allowed per window. <br>  * RateLimit-Remaining - The number of requests remaining in the current window (zero). <br>  * RateLimit-Reset - Seconds until the current window resets. <br>  * Retry-After - Seconds to wait before retrying. <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

