<script lang="ts">
  import { ToastNotification } from "carbon-components-svelte";
  import { notificationstore } from "../store/notifications";
</script>

<div class="notification-region" aria-live="polite" aria-label="Notifications">
  {#each $notificationstore as notification (notification.id)}
    <ToastNotification
      kind={notification.kind}
      title={notification.title}
      subtitle={notification.subtitle}
      timeout={notification.timeout}
      on:close={() => notificationstore.remove(notification)}
    />
  {/each}
</div>
