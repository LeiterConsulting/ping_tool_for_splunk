# Ping Tool for Splunk - Release v5.4.0

## Accuracy and state

- Schema v2 distinguishes a valid no-reply observation from a probe/backend failure. Invalid measurements omit packet loss and latency instead of reporting false zero values.
- Latency retains sub-millisecond precision and every attempt carries sequence, send/receive timestamps, backend, collector ID, endpoint ID, cycle ID, and event ID.
- Health uses configurable down/recovery hysteresis and a restart-safe checkpoint. One missed cycle no longer immediately declares an endpoint down.

## Performance and reliability

- Windows uses `IcmpSendEcho` through the native IP Helper API, eliminating four `ping.exe` process launches per endpoint cycle.
- Probe deadlines are bounded to the actual batch duration, and targets are paced across the configured cycle rather than launched in one burst.
- File, HEC, and metrics output runs through a bounded asynchronous queue so network output latency does not distort probe timing.
- A deployment lock prevents multiple monitor engines from using the same config concurrently.

## Splunk compatibility

- Ping Monitor app 2.8.0 adds `ping_normalize`, which reads mixed v1/v2 history, deduplicates v2 by event identity, and normalizes legacy summary data to one endpoint observation per minute.
- Current-state views group by stable endpoint ID and use the v2 hysteretic state while retaining packet-loss inference for v1 history.
