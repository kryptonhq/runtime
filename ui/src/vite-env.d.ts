/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_KRYPTON_VERSION?: string;
  readonly VITE_KRYPTON_COMMIT?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
