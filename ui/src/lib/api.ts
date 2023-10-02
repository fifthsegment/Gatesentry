// const apiBaseUrl = import.meta.env.VITE_APP_API_BASE_URL;
const env = import.meta.env;

class AppAPI {
  baseURL: string;
  headers: Record<string, string>;
  onUnauthorizedcallMe: () => void;
  loggedIn: boolean;
  jwtToken: string;

  constructor() {
    const jwt = localStorage.getItem("jwt");
    this.baseURL = "/api";

    this.headers = {
      Authorization: `Bearer ${jwt}`,
    };
    this.onUnauthorizedcallMe = () => {
      // window.location.hash = "#/loggedOut";
    };
  }

  verifyToken(): Promise<boolean> {
    return this.doCallRaw("/auth/verify").then((response) => {
      if (response.status === 200) {
        this.jwtToken = localStorage.getItem("jwt") || "";
        this.setLoggedIn(this.jwtToken);
        return true;
      } else {
        return false;
      }
    });
  }

  setLoggedIn(jwtToken: string) {
    this.jwtToken = jwtToken;
    this.loggedIn = true;
  }

  setLoggedOut() {
    localStorage.removeItem("jwt");
    this.loggedIn = false;
  }

  onUnauthorized(callMe: () => void) {
    this.onUnauthorizedcallMe = callMe;
  }

  setHeaders(headers: Record<string, string>) {
    this.headers = headers;
  }

  getHeaders(): Record<string, string> {
    const jwt = localStorage.getItem("jwt");
    this.headers.Authorization = `Bearer ${jwt}`;
    return this.headers;
  }

  doCallRaw(
    endpoint = "/",
    method = "get",
    dataTosend = {},
  ): Promise<Response> {
    const that = this;
    const additionalHeaders = {
      method: method,
      headers: this.getHeaders(),
    };
    if (method === "post") {
      // @ts-ignore
      additionalHeaders.body = JSON.stringify(dataTosend);
    }
    const p = new Promise<Response>((resolve, reject) => {
      fetch(that.baseURL + endpoint, additionalHeaders)
        .then((response) => {
          resolve(response);
        })
        .catch((err) => {
          reject(err);
        });
    });
    return p;
  }

  async doCall(endpoint = "/", method = "get", dataTosend = {}): Promise<any> {
    const that = this;
    const additionalHeaders = {
      method: method,
      headers: this.getHeaders(),
    };
    if (method === "post") {
      // @ts-ignore
      additionalHeaders.body = JSON.stringify(dataTosend);
    }
    try {
      const response = await fetch(that.baseURL + endpoint, additionalHeaders);
      if (response.status === 401) {
        that.onUnauthorizedcallMe();
      } else if (response.status === 200) {
        const json = await response.json();
        return json;
      }
    } catch (err) {
      console.error("Gatesentry API error : ", err);
      throw err;
    }
  }
}

export default AppAPI;
