import AgentsTable, {
  imageCell,
  modeCell,
  nameCell,
  namespaceCell,
  phaseCell,
  replicasCell,
} from "../components/AgentsTable";

export default function AgentsList() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Agents</h1>
        <p className="text-sm text-slate-500 mt-1">
          A2A and HTTP agents registered across the cluster.
        </p>
      </div>

      <AgentsTable
        excludeProtocol="mcp"
        detailHref={(a) => `/agents/${a.namespace}/${a.name}`}
        columns={[
          {
            field: "name",
            label: "Name",
            cell: nameCell((a) => `/agents/${a.namespace}/${a.name}`),
          },
          { field: "namespace", label: "Namespace", cell: namespaceCell },
          { field: null, label: "Mode", cell: modeCell },
          { field: "phase", label: "Phase", cell: phaseCell },
          { field: "replicas", label: "Replicas", cell: replicasCell },
          { field: "image", label: "Image", cell: imageCell },
        ]}
      />
    </div>
  );
}
