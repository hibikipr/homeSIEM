<script lang="ts">
  import { heatTierColor } from '$lib/wall';

  let { rows }: { rows: { source: string; hours: string[] }[] } = $props();
</script>

<div class="heat-grid">
  {#each rows as row (row.source)}
    <div class="row">
      <span class="label">{row.source}</span>
      <div class="cells">
        {#each row.hours as tier, i (i)}
          <span class="cell" style="background: {heatTierColor(tier)}" title={tier}></span>
        {/each}
      </div>
    </div>
  {/each}
</div>

<style>
  .heat-grid {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .label {
    width: 96px;
    font-family: var(--font-mono);
    font-size: var(--text-label);
    color: var(--color-muted);
    flex-shrink: 0;
  }
  .cells {
    display: flex;
    gap: 3px;
    flex: 1;
  }
  .cell {
    flex: 1;
    height: 19px;
    border-radius: 3px;
  }
</style>
