import time
import urllib.request
from requests_html import HTMLSession
import time


def measure_performance(proxy_server_url, test_url):
    """Measures the performance of a proxy server.

    Args:
      proxy_server_url: The URL of the proxy server.
      test_url: The URL of the website to test the proxy server against.

    Returns:
      A tuple of (response_time, status_code).
    """
    session = HTMLSession()
    proxies = {
        "http": proxy_server_url,
        "https": proxy_server_url,
    }
    session.proxies = proxies

    start_time = time.time()
    try:
        response = session.get(test_url, verify=False)
        response.html.render()  # This will download the assets and execute JavaScript
        status_code = response.status_code
    except Exception as e:
        print(f"An error occurred: {e}")
        status_code = None
    finally:
        response_time = time.time() - start_time

    return response_time, status_code


def main():
    """Measures the performance of a proxy server and prints the results to the console."""

    proxy_server_url = "http://guest:password@10.1.0.141:10413"
    test_url = "https://nrk.no"

    response_time, status_code = measure_performance(
        proxy_server_url, test_url)

    print("Response time:", response_time)
    print("Status code:", status_code)


if __name__ == "__main__":
    main()
