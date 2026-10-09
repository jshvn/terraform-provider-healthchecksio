# Import with the unique_key. The unique_key is not a ping credential.
terraform import healthchecksio_check.backup a6c7b0a8a66bed0df66abfdab3c77736861703ee

# Import with the UUID. The UUID is the ping credential. Do not put it in a file in git.
terraform import healthchecksio_check.backup 31365bce-8da9-4729-8ff3-aaa71d56b712
