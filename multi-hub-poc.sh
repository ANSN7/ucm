# create 2 hub clusters
# kind create cluster --name hub1
# kind create cluster --name hub2

# install hub on hub1
clusteradm init --wait --context kind-hub1

OUTPUT=$(clusteradm get token --context kind-hub1 | tail -1)
JOIN_COMMAND=$(echo $OUTPUT | sed 's/<cluster_name>/hub2/')

# join hub2 to hub1
eval "$JOIN_COMMAND --force-internal-endpoint-lookup --context kind-hub2"
sleep 60

# accept join request from hub1
clusteradm accept --clusters hub2 --context kind-hub1
sleep 2



# install hub on hub2
clusteradm init --wait --context kind-hub2

OUTPUT=$(clusteradm get token --context kind-hub2 | tail -1)
JOIN_COMMAND=$(echo $OUTPUT | sed 's/<cluster_name>/hub1/')

# join hub1 to hub2
eval "$JOIN_COMMAND --force-internal-endpoint-lookup --context kind-hub1"
sleep 60

# accept join request from hub1
clusteradm accept --clusters hub1 --context kind-hub2
sleep 2


# MANIFEST WORK
# clusteradm create work my-first-work -f nginx.yaml --context kind-hub1 --clusters kind-hub2
# clusteradm create work my-first-work -f nginx.yaml --placement default/placement1 --context kind-hub


# CLUSTER SETS
# clusteradm --context kind-hub1 get clustersets
# clusteradm clusterset bind default --namespace default  --context=kind-hub1