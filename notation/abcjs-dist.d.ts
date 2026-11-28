// The prebuilt bundle the wrapper imports (abcjs.ts): a UMD file that Bun loads
// as CommonJS, so the module's exports are its default export. Its API is the
// one the package types under the module name "abcjs" (types/index.d.ts).
declare module "abcjs/dist/abcjs-basic-min.js" {
  const abcjs: typeof import("abcjs");
  export default abcjs;
}
