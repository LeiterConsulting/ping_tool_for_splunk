[ping_data]
# Index containing ping event data
events_index = <string>
* The Splunk index where ping event data is stored
* Default: main

# Sourcetype for ping events
events_sourcetype = <string>
* The sourcetype used for ping monitor events
* Default: ping_monitor

# Metrics index (metrics-type index for mstats)
metrics_index = <string>
* The metrics index where ping metrics data is stored
* Used for mstats queries
* Default: ping_metrics

# Default span for metrics aggregation
metrics_span = <string>
* Time span for metrics aggregation
* Default: 1m

[correlation]
# Enable automatic discovery of related data sources
auto_discover = <bool>
* Enable automatic discovery of indexes with matching assets
* Default: true

# Indexes to search for correlated data
correlation_indexes = <string>
* Comma-separated list of indexes to search for correlated data
* Use * for all indexes, or leave empty for auto-discovery
* Default: empty

# Maximum age of data to consider for correlation discovery
discovery_timerange = <string>
* Time range to search when discovering correlated data sources
* Default: -24h

# Fields to use for asset matching
match_fields = <string>
* Comma-separated list of fields to use when matching assets
* These fields will be checked for IP addresses and hostnames
* Default: ip,hostname,host,src_ip,dest_ip,src,dest,src_host,dest_host,dvc,dvc_ip
