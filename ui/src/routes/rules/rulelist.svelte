<script lang="ts">
  import { Button, Column, InlineLoading, InlineNotification, Row } from "carbon-components-svelte";
  import Ruleform from "./rform.svelte";
  import { Add } from "carbon-icons-svelte";
  import { onMount } from "svelte";

  let rules = [];
  let loading = false;
  let error = "";
  let success = "";
  let expandedRules = new Set(); // Track which rules are expanded

  const API_BASE = "/api/rules";

  function toggleExpand(index) {
    if (expandedRules.has(index)) {
      expandedRules.delete(index);
    } else {
      expandedRules.add(index);
    }
    expandedRules = expandedRules; // Trigger reactivity
  }

  async function loadRules() {
    loading = true;
    error = "";
    try {
      const token = localStorage.getItem("jwt");
      if (!token) {
        throw new Error("Please login first to view rules");
      }
      const response = await fetch(API_BASE, {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      });
      if (response.status === 401) {
        throw new Error("Authentication failed. Please login again");
      }
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`Failed to load rules: ${response.status} - ${errorText}`);
      }
      const data = await response.json();
      rules = data.rules || [];
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    loadRules();
  });

  function addRule() {
    console.log("add rule");
    const newIndex = rules.length;
    rules = [
      ...rules,
      {
        id: "",
        name: `Rule ${rules.length + 1}`,
        enabled: true,
        priority: rules.length,
        domain: "",
        action: "allow",
        mitm_action: "enable",
        block_type: "none",
        blocked_content_types: [],
        url_regex_patterns: [],
        time_restriction: null,
        users: [],
        description: "",
      },
    ];
    // Auto-expand newly added rule
    expandedRules.add(newIndex);
    expandedRules = expandedRules;
  }

  async function removeRule(e) {
    const index = e.detail;
    const rule = rules[index];
    
    // If the rule has an ID, delete it from the backend
    if (rule.id) {
      loading = true;
      error = "";
      success = "";

      try {
        const token = localStorage.getItem("jwt");
        const response = await fetch(`${API_BASE}/${rule.id}`, {
          method: "DELETE",
          headers: {
            Authorization: `Bearer ${token}`,
          },
        });

        if (!response.ok) {
          const errorData = await response.text();
          throw new Error(`Failed to delete rule: ${errorData}`);
        }

        success = `Rule "${rule.name || `#${index + 1}`}" deleted successfully!`;
        
        // Clear success message after 3 seconds
        setTimeout(() => {
          success = "";
        }, 3000);
      } catch (err) {
        error = err.message;
        loading = false;
        return; // Don't remove from UI if delete failed
      } finally {
        loading = false;
      }
    }

    // Remove from local array
    console.log("Removing rule", index);
    rules = rules.filter((_, i) => i !== index);
    // Clean up expanded state
    expandedRules.delete(index);
    expandedRules = expandedRules;
  }

  async function saveRule(e) {
    const index = e.detail;
    loading = true;
    error = "";
    success = "";

    try {
      const token = localStorage.getItem("jwt");
      const rule = rules[index];
      const url = rule.id ? `${API_BASE}/${rule.id}` : API_BASE;
      const method = rule.id ? "PUT" : "POST";

      const response = await fetch(url, {
        method,
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(rule),
      });

      if (!response.ok) {
        const errorData = await response.text();
        throw new Error(`Failed to save rule: ${errorData}`);
      }

      const savedRule = await response.json();
      // Update the rule with the response (gets ID if newly created)
      rules[index] = savedRule.rule || savedRule;
      rules = rules; // Trigger reactivity

      success = `Rule "${rule.name || `#${index + 1}`}" saved successfully!`;
      
      // Collapse the rule after successful save
      expandedRules.delete(index);
      expandedRules = expandedRules;

      // Clear success message after 3 seconds
      setTimeout(() => {
        success = "";
      }, 3000);
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }

  async function saveRules() {
    loading = true;
    error = "";
    success = "";

    try {
      const token = localStorage.getItem("jwt");

      // Save each rule (create or update)
      for (const rule of rules) {
        const url = rule.id ? `${API_BASE}/${rule.id}` : API_BASE;
        const method = rule.id ? "PUT" : "POST";

        const response = await fetch(url, {
          method,
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${token}`,
          },
          body: JSON.stringify(rule),
        });

        if (!response.ok) {
          const errorData = await response.text();
          throw new Error(`Failed to save rule: ${errorData}`);
        }
      }

      success = "All rules saved successfully!";
      // Reload rules to get updated IDs
      await loadRules();
      
      // Clear success message after 3 seconds
      setTimeout(() => {
        success = "";
      }, 3000);
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }
</script>

<Row>
  <Column>
    {#if error}
      <InlineNotification
        kind="error"
        title="Error"
        subtitle={error}
        on:close={() => (error = "")}
      />
    {/if}

    {#if success}
      <InlineNotification
        kind="success"
        title="Success"
        subtitle={success}
        on:close={() => (success = "")}
      />
    {/if}

    <div style="display: flex; justify-content: flex-end; margin-bottom: 15px;">
      <Button on:click={addRule} icon={Add} size="small">Add Rule</Button>
    </div>

    {#if loading}
      <InlineLoading description="Loading rules..." />
    {:else}
      {#each rules as rule, index (index)}
        <Ruleform 
          {rule} 
          {index} 
          expanded={expandedRules.has(index)}
          on:toggle={() => toggleExpand(index)}
          on:remove={removeRule}
          on:save={saveRule}
        />
      {/each}
    {/if}
  </Column>
</Row>
