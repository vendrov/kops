Thanks for submitting an issue! Please fill in as much of the template below as
you can.

------------- BUG REPORT TEMPLATE --------------------

1. What `kops` version are you running? The command `kops version`, will display
 this information.

2. What Kubernetes version are you running? `kubectl version` will print the
 version if a cluster is running or provide the Kubernetes version specified as
 a `kops` flag.

3. What cloud provider are you using?

4. What commands did you run?  What is the simplest way to reproduce this issue?

5. What happened after the commands executed?

6. What did you expect to happen?

7. Please provide your cluster manifest. Execute
  `kops get --name my.example.com -oyaml` to display your cluster manifest.
  You may want to remove your cluster name and other sensitive information.

8. Please run the commands with most verbose logging by adding the `-v 10` flag.
  Paste the logs into this report, or in a gist and provide the gist link here.

9. Anything else do we need to know?


------------- FEATURE REQUEST TEMPLATE --------------------

1. Describe IN DETAIL the feature/behavior/change you would like to see.

2. Feel free to provide a design supporting your feature request.
