import { useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { useNavigate } from "react-router-dom";

import { Button } from "../components/ui/button";
import { HexagonBackground } from "../components/ui/hexagon-background";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../components/ui/card";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
} from "../components/ui/field";
import { Input } from "../components/ui/input";
import { api } from "../lib/api";

const providerLabels: Record<string, string> = {
  github: "GitHub",
  linuxdo: "LinuxDo",
};

function base64urlToBuffer(b64: string): ArrayBuffer {
  const base64 = b64.replace(/-/g, "+").replace(/_/g, "/");
  const pad = base64.length % 4 === 0 ? "" : "=".repeat(4 - (base64.length % 4));
  const bin = atob(base64 + pad);
  const bytes = new Uint8Array(bin.length);

  for (let i = 0; i < bin.length; i++) {
    bytes[i] = bin.charCodeAt(i);
  }

  return bytes.buffer;
}

function bufferToBase64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let bin = "";

  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i]);
  }

  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
}

export function LoginPage() {
  const navigate = useNavigate();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [oauthProviders, setOauthProviders] = useState<string[]>([]);

  useEffect(() => {
    api
      .oauthProviders()
      .then((data) => setOauthProviders(data.providers || []))
      .catch(() => {});
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      if (mode === "register") {
        await api.register(username, password);
      } else {
        await api.login(username, password);
      }

      navigate("/dashboard");
    } catch (err: any) {
      setError(err.message);
    }

    setLoading(false);
  }

  async function handlePasskeyLogin() {
    setError("");
    setLoading(true);

    try {
      const options = await fetch("/api/auth/passkey/login/begin", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      }).then((r) => r.json());

      options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);

      if (options.publicKey.allowCredentials) {
        options.publicKey.allowCredentials = options.publicKey.allowCredentials.map(
          (credential: any) => ({
            ...credential,
            id: base64urlToBuffer(credential.id),
          }),
        );
      }

      const credential = (await navigator.credentials.get(options)) as PublicKeyCredential;

      if (!credential) {
        throw new Error("cancelled");
      }

      const response = credential.response as AuthenticatorAssertionResponse;
      const body = JSON.stringify({
        id: credential.id,
        rawId: bufferToBase64url(credential.rawId),
        type: credential.type,
        response: {
          authenticatorData: bufferToBase64url(response.authenticatorData),
          clientDataJSON: bufferToBase64url(response.clientDataJSON),
          signature: bufferToBase64url(response.signature),
          userHandle: response.userHandle ? bufferToBase64url(response.userHandle) : "",
        },
      });

      const res = await fetch("/api/auth/passkey/login/finish", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body,
      });

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || "login failed");
      }

      navigate("/dashboard");
    } catch (err: any) {
      if (err.name !== "NotAllowedError") {
        setError(err.message || "Passkey 登录失败");
      }
    }

    setLoading(false);
  }

  async function handlePasskeyRegister() {
    if (!username.trim()) {
      setError("请先输入用户名");
      return;
    }

    setError("");
    setLoading(true);

    try {
      const options = await fetch("/api/auth/passkey/register/begin", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim() }),
      }).then(async (r) => {
        if (!r.ok) {
          throw new Error((await r.json()).error);
        }

        return r.json();
      });

      options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
      options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id);

      if (options.publicKey.excludeCredentials) {
        options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(
          (credential: any) => ({
            ...credential,
            id: base64urlToBuffer(credential.id),
          }),
        );
      }

      const credential = (await navigator.credentials.create(options)) as PublicKeyCredential;

      if (!credential) {
        throw new Error("cancelled");
      }

      const response = credential.response as AuthenticatorAttestationResponse;
      const body = JSON.stringify({
        id: credential.id,
        rawId: bufferToBase64url(credential.rawId),
        type: credential.type,
        response: {
          attestationObject: bufferToBase64url(response.attestationObject),
          clientDataJSON: bufferToBase64url(response.clientDataJSON),
        },
      });

      const res = await fetch(
        `/api/auth/passkey/register/finish?username=${encodeURIComponent(username.trim())}`,
        {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body,
        },
      );

      if (!res.ok) {
        const data = await res.json();
        throw new Error(data.error || "register failed");
      }

      navigate("/dashboard");
    } catch (err: any) {
      if (err.name !== "NotAllowedError") {
        setError(err.message || "Passkey 注册失败");
      }
    }

    setLoading(false);
  }

  function handleOAuth(provider: string) {
    window.location.href = `/api/auth/oauth/${provider}`;
  }

  function toggleMode() {
    setMode(mode === "login" ? "register" : "login");
    setError("");
  }

  const supportsPasskey = typeof window !== "undefined" && "PublicKeyCredential" in window;

  return (
    <div className="relative isolate flex min-h-screen items-center justify-center overflow-x-hidden bg-background px-6 py-12 sm:px-8 sm:py-16">
      <HexagonBackground className="opacity-55" hexagonSize={78} hexagonMargin={5} />
      <div className="absolute inset-x-0 top-0 h-[28rem] bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.14),transparent_42%)]" />

      <div className="relative z-10 w-full max-w-md">
        <div className="flex flex-col gap-10">
          <div className="space-y-4 text-center">
            <p className="text-sm font-medium tracking-[0.18em] text-muted-foreground uppercase">
              OpeniLink Hub
            </p>
            <div className="space-y-3">
              <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
                {mode === "login" ? "欢迎回来" : "创建你的账号"}
              </h1>
              <p className="text-base leading-7 text-muted-foreground">
                {mode === "login"
                  ? "登录后即可管理 Bot、渠道和 Webhook 插件。"
                  : "注册后就可以开始配置微信 Bot 和消息路由。"}
              </p>
            </div>
          </div>

          <Card className="rounded-[1.75rem] border-white/8 bg-card/82 backdrop-blur-sm">
            <CardHeader className="px-6 pt-8 pb-4 text-center sm:px-8">
              <CardTitle className="text-2xl">OpeniLink Hub</CardTitle>
              <CardDescription>{mode === "login" ? "登录你的账号" : "创建新账号"}</CardDescription>
            </CardHeader>
            <CardContent className="px-6 pb-8 sm:px-8">
              <form onSubmit={handleSubmit}>
                <FieldGroup className="gap-6">
                  {oauthProviders.length > 0 && (
                    <>
                      <Field>
                        {oauthProviders.map((provider) => (
                          <Button
                            key={provider}
                            type="button"
                            variant="outline"
                            className="h-10 w-full text-sm"
                            onClick={() => handleOAuth(provider)}
                            disabled={loading}
                          >
                            使用 {providerLabels[provider] || provider} 登录
                          </Button>
                        ))}
                      </Field>
                      <FieldSeparator className="*:data-[slot=field-separator-content]:bg-card">
                        或继续使用账号
                      </FieldSeparator>
                    </>
                  )}

                  <Field>
                    <FieldLabel htmlFor="username">用户名</FieldLabel>
                    <Input
                      id="username"
                      placeholder="请输入用户名"
                      className="h-10 text-base"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoFocus
                      disabled={loading}
                      required
                    />
                  </Field>

                  <Field>
                    <div className="flex items-center">
                      <FieldLabel htmlFor="password">密码</FieldLabel>
                      {mode === "register" && (
                        <span className="ml-auto text-sm text-muted-foreground">至少 8 位</span>
                      )}
                    </div>
                    <Input
                      id="password"
                      type="password"
                      placeholder={mode === "login" ? "请输入密码" : "设置登录密码"}
                      className="h-10 text-base"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      disabled={loading}
                      required
                    />
                  </Field>

                  <FieldError>{error}</FieldError>

                  {supportsPasskey && (
                    <>
                      <FieldSeparator className="*:data-[slot=field-separator-content]:bg-card">
                        {mode === "login" ? "或使用 Passkey" : "或直接创建 Passkey"}
                      </FieldSeparator>
                      <Field>
                        <Button
                          type="button"
                          variant="outline"
                          className="h-10 w-full text-sm"
                          onClick={mode === "login" ? handlePasskeyLogin : handlePasskeyRegister}
                          disabled={loading || (mode === "register" && !username.trim())}
                        >
                          <KeyRound className="mr-2 h-4 w-4" />
                          {mode === "login" ? "使用 Passkey 登录" : "使用 Passkey 注册（无需密码）"}
                        </Button>
                        {mode === "register" && (
                          <FieldDescription className="text-center">
                            Passkey 注册会使用上方填写的用户名创建账号。
                          </FieldDescription>
                        )}
                      </Field>
                    </>
                  )}

                  <Field>
                    <Button type="submit" className="w-full" disabled={loading}>
                      {loading ? "..." : mode === "login" ? "登录" : "注册"}
                    </Button>
                    <FieldDescription className="text-center">
                      {mode === "login" ? "没有账号？" : "已有账号？"}{" "}
                      <button
                        type="button"
                        className="font-medium text-foreground underline underline-offset-4"
                        onClick={toggleMode}
                      >
                        {mode === "login" ? "注册" : "登录"}
                      </button>
                    </FieldDescription>
                  </Field>
                </FieldGroup>
              </form>
            </CardContent>
          </Card>

          <FieldDescription className="px-6 pt-1 text-center text-sm leading-6">
            支持密码、Passkey 和 OAuth 登录方式。
          </FieldDescription>
        </div>
      </div>
    </div>
  );
}
