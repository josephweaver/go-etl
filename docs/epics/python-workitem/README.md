# Python WorkItem Epic

Create a worker WorkItem Plugin to handle the execution of a generic python script.

- It should be able to create an environment JIT to execute the script. 
    - As an input we need an Env specification 
    - or somehow determine what libraries are needed, maybe scan the scripts
- It python script should be accessible by client application.  When submited the client copies these as part of the workflow submission.
- The worker container should have severval commonly used python packages cached for quick creation of environments
- When executing we need to capture console out/err to a logging framework.
- To execute, we should be able to pass in several command line variables.




