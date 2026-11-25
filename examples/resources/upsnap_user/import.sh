# By record ID
terraform import upsnap_user.alice 27do0wbcuyfmbmx

# By username, which UpSnap keeps unique
terraform import upsnap_user.alice username:alice
