import EmptyState from "../components/EmptyState";

interface Props {
    title: string;
    description?: string;
}

// PlaceholderRoute marks a route that's wired into the nav but whose
// page component lands in a later step (HostsPage in step 6, SessionsPage
// in step 8). Lets the new shell ship without dangling NavLinks.
export default function PlaceholderRoute({ title, description }: Props) {
    return (
        <EmptyState
            title={title}
            description={
                description ??
                "This view ships in the next step of the IA rebuild. Sidebar nav already routes here."
            }
            fill
        />
    );
}
