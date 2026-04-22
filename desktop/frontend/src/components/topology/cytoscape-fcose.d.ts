// Minimal ambient declaration for cytoscape-fcose. The upstream
// package ships no types, and the public surface we use is just the
// default export registered via cytoscape.use().
declare module "cytoscape-fcose" {
    import type cytoscape from "cytoscape";
    const fcose: cytoscape.Ext;
    export default fcose;
}
