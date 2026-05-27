import AgentsTable, {
  imageCell,
  nameCell,
  namespaceCell,
  phaseCell,
  replicasCell,
} from "../components/AgentsTable";

export default function MCPList() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">MCP servers</h1>
        <p className="text-sm text-slate-500 mt-1">
          Agents speaking the Model Context Protocol. Click one to introspect
          and call its tools.
        </p>
      </div>

      <AgentsTable
        protocol="mcp"
        detailHref={(a) => `/mcp/${a.namespace}/${a.name}`}
        columns={[
          {
            field: "name",
            label: "Name",
            cell: nameCell((a) => `/mcp/${a.namespace}/${a.name}`),
          },
          { field: "namespace", label: "Namespace", cell: namespaceCell },
          { field: "phase", label: "Phase", cell: phaseCell },
          { field: "replicas", label: "Replicas", cell: replicasCell },
          { field: "image", label: "Image", cell: imageCell },
        ]}
      />
    </div>
  );
}
