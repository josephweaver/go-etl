# Logging Framework

Loggings is serious business in orcastration.  With out a strategy we can easily end up with thousands of logs all over the place.  Ideally each level (client, controller, Worker, Worker Subprocess) will report back up the chain via some logging hook.

## Informational Logging

At this level, we log broad concepts such as anytime we cross an application bounrdry.  E.g. Client logs "Calling Controller Start up", "Submitting workflow to Controller",  

Controller might log "Compileing Workflow",  "Worker 492 requested work", "Sending WorkItem 48920349 to Worker 492." "Worker 492 Acknowledged Workitem 48920349."  "Worker reports workitem 48920349 complete.". 

Workers might log "Requesting Work",  "WorkItem 48920349 received, begin processing."  "WorkItem 48920349 complete, sending status update."  "Controller Acknowledge 48920349 complete".  

Worker Subproccess might log, "  Copying files from x to y", "  Copied 104 files from x to y.",  "  Executing script myscript.py"  "[script] line 1", "[script] line 2",..., "  myscript.py executed sucessfully return code 0."

## Verbose Logging

