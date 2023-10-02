<script lang="ts">
  import { afterUpdate } from "svelte";
  import { notificationstore } from "../store/notifications";
  import { ToastNotification } from "carbon-components-svelte";

  let notifications = [];

  afterUpdate(() => {
    notifications = $notificationstore;
  });
</script>

<div style="position: absolute; right:0; bottom: 0; text-align:left;">
  {#if notifications.length > 0}
    {#each notifications as notification}
      <ToastNotification
        kind={notification.kind}
        title={notification.title}
        subtitle={notification.subtitle}
        timeout={notification.timeout}
        on:close={(e) => {
          notificationstore.remove(notification);
        }}
      />
    {/each}
  {/if}
</div>
