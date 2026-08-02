<script lang="ts">
  import '@phosphor-icons/web/regular';
  import '$lib/styles/tokens.css';
  import Nav from '$lib/components/Nav.svelte';
  import type { LayoutData } from './$types';
  import type { Snippet } from 'svelte';

  let { data, children }: { data: LayoutData; children: Snippet } = $props();
</script>

<Nav
  activeRoute={data.activeRoute}
  alertCount={0}
  ingestRate={0}
  userDisplayName={data.user?.displayName ?? ''}
  userRole={data.user?.role ?? ''}
/>

<main>
  {@render children()}
</main>
