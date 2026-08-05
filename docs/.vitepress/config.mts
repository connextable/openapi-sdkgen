import { defineConfig } from "vitepress";

const englishTheme = {
  logo: "/mark.svg",
  nav: [
    { text: "Start", link: "/guide/getting-started" },
    { text: "Guides", link: "/guide/client" },
    { text: "Reference", link: "/reference/cli" },
  ],
  sidebar: {
    "/guide/": [
      {
        text: "Build and use your SDK",
        items: [
          { text: "Create your first SDK", link: "/guide/getting-started" },
          { text: "Generate from OpenAPI", link: "/guide/generate" },
          { text: "Call your API", link: "/guide/client" },
          { text: "Authentication, transport, and streams", link: "/guide/transport" },
          { text: "Receive webhooks and callbacks", link: "/guide/server" },
        ],
      },
    ],
    "/reference/": [
      {
        text: "Reference",
        items: [
          { text: "CLI", link: "/reference/cli" },
          { text: "Generated client API", link: "/reference/client-api" },
          { text: "TypeScript types", link: "/reference/typescript-types" },
          { text: "OpenAPI support", link: "/reference/capabilities" },
          { text: "SDK extensions", link: "/reference/extensions" },
        ],
      },
    ],
  },
  search: { provider: "local" },
  outline: { level: [2, 3], label: "On this page" },
  editLink: {
    pattern: "https://github.com/jinyongp/openapi-sdkgen/edit/main/docs/:path",
    text: "Edit this page on GitHub",
  },
  footer: {
    message: "Apache License 2.0 · © Jinyong Park",
  },
};

const koreanTheme = {
  logo: "/mark.svg",
  nav: [
    { text: "시작하기", link: "/ko/guide/getting-started" },
    { text: "사용 가이드", link: "/ko/guide/client" },
    { text: "레퍼런스", link: "/ko/reference/cli" },
  ],
  sidebar: {
    "/ko/guide/": [
      {
        text: "SDK 만들기와 사용하기",
        items: [
          { text: "첫 SDK 만들기", link: "/ko/guide/getting-started" },
          { text: "OpenAPI에서 SDK 생성", link: "/ko/guide/generate" },
          { text: "클라이언트로 API 호출", link: "/ko/guide/client" },
          { text: "인증·전송·스트림", link: "/ko/guide/transport" },
          { text: "Webhook과 Callback 수신", link: "/ko/guide/server" },
        ],
      },
    ],
    "/ko/reference/": [
      {
        text: "레퍼런스",
        items: [
          { text: "CLI", link: "/ko/reference/cli" },
          { text: "생성된 클라이언트 API", link: "/ko/reference/client-api" },
          { text: "TypeScript 타입", link: "/ko/reference/typescript-types" },
          { text: "OpenAPI 지원 범위", link: "/ko/reference/capabilities" },
          { text: "SDK 확장 기능", link: "/ko/reference/extensions" },
        ],
      },
    ],
  },
  search: {
    provider: "local",
    options: {
      translations: {
        button: {
          buttonText: "검색",
          buttonAriaLabel: "검색",
        },
        modal: {
          displayDetails: "상세 보기",
          resetButtonTitle: "검색 초기화",
          backButtonTitle: "닫기",
          noResultsText: "결과 없음",
          footer: {
            selectText: "선택",
            selectKeyAriaLabel: "Enter",
            navigateText: "이동",
            navigateUpKeyAriaLabel: "위쪽 화살표",
            navigateDownKeyAriaLabel: "아래쪽 화살표",
            closeText: "닫기",
            closeKeyAriaLabel: "Escape",
          },
        },
      },
    },
  },
  outline: { level: [2, 3], label: "이 페이지에서" },
  editLink: {
    pattern: "https://github.com/jinyongp/openapi-sdkgen/edit/main/docs/:path",
    text: "GitHub에서 이 페이지 수정",
  },
  footer: {
    message: "Apache License 2.0 · © Jinyong Park",
  },
};

export default defineConfig({
  lang: "en-US",
  title: "openapi-sdkgen",
  description: "Generate application SDK source from OpenAPI 3.x documents.",
  base: "/openapi-sdkgen/",
  srcExclude: [
    "openapi-feature-inventory.md",
    "openapi-feature-matrix.md",
    "openapi-sdkgen-implementation-roadmap.md",
  ],
  lastUpdated: true,
  themeConfig: englishTheme,
  locales: {
    root: {
      label: "English",
      lang: "en-US",
      themeConfig: englishTheme,
    },
    ko: {
      label: "한국어",
      lang: "ko-KR",
      link: "/ko/",
      title: "openapi-sdkgen",
      description: "OpenAPI 3.x 문서에서 애플리케이션 SDK 소스를 생성.",
      themeConfig: koreanTheme,
    },
  },
});
