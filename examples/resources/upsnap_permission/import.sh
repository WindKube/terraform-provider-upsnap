# By record ID
terraform import upsnap_permission.alice nmj3ko20gzkg8n3

# By user, since every user has at most one permission record
terraform import upsnap_permission.alice user:27do0wbcuyfmbmx
terraform import upsnap_permission.alice username:alice
